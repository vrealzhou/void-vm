package vmctl

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

type bootstrapScriptData struct {
	GuestUser              string
	GuestHome              string
	DefaultShell           string
	DefaultEditor          string
	EditorCmd              string
	WindowManager          string
	StarshipPresetURL      string
	HomebrewPrefix         string
	BootstrapBrewPackages  string
	BootstrapCargoPackages string
	GitUserName            string
	GitUserEmail           string
	Timezone               string
	VoidRepoURL            string
	ExtraCommands          string
	SetDefaultShell        bool
}

func editorCmd(editor string) string {
	switch editor {
	case "helix":
		return "hx"
	default:
		return "nvim"
	}
}

func generateBootstrapScript(cfg Config) (string, error) {
	data := bootstrapScriptData{
		GuestUser:              cfg.GuestUser,
		GuestHome:              "/home/" + cfg.GuestUser,
		DefaultShell:           cfg.DefaultShell,
		DefaultEditor:          cfg.DefaultEditor,
		EditorCmd:              editorCmd(cfg.DefaultEditor),
		WindowManager:          cfg.WindowManager,
		StarshipPresetURL:      cfg.StarshipPresetURL,
		HomebrewPrefix:         "/home/linuxbrew/.linuxbrew",
		BootstrapBrewPackages:  cfg.BootstrapBrewPackages,
		BootstrapCargoPackages: cfg.BootstrapCargoPackages,
		GitUserName:            cfg.GitUserName,
		GitUserEmail:           cfg.GitUserEmail,
		Timezone:               cfg.Timezone,
		VoidRepoURL:            strings.TrimRight(cfg.VoidRepository, "/") + "/current/aarch64",
		ExtraCommands:          strings.ReplaceAll(cfg.BootstrapExtraCommands, "\n", " && "),
		SetDefaultShell:        cfg.SetDefaultShell,
	}

	t := template.Must(template.New("bootstrap").Parse(bootstrapTemplate))
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to generate bootstrap script: %w", err)
	}
	return buf.String(), nil
}

const bootstrapTemplate = `#!/usr/bin/env bash

set -euo pipefail

TARGET_USER="{{.GuestUser}}"
TARGET_HOME="{{.GuestHome}}"
DEFAULT_SHELL="{{.DefaultShell}}"
DEFAULT_EDITOR="{{.DefaultEditor}}"
EDITOR_CMD="{{.EditorCmd}}"
WINDOW_MANAGER="{{.WindowManager}}"
STARSHIP_PRESET_URL="{{.StarshipPresetURL}}"
HOMEBREW_PREFIX="{{.HomebrewPrefix}}"
BOOTSTRAP_XBPS_REPOSITORY="{{.VoidRepoURL}}"
BOOTSTRAP_TIMEZONE="{{.Timezone}}"
BOOTSTRAP_BREW_PACKAGES="{{.BootstrapBrewPackages}}"
BOOTSTRAP_CARGO_PACKAGES="{{.BootstrapCargoPackages}}"
GIT_USER_NAME="{{.GitUserName}}"
GIT_USER_EMAIL="{{.GitUserEmail}}"

STARSHIP_CONFIG_PATH="${TARGET_HOME}/.config/starship.toml"
FISH_CONFIG_DIR="${TARGET_HOME}/.config/fish"
FISH_CONFIG_SNIPPET="${FISH_CONFIG_DIR}/conf.d/vmctl-shell.fish"
FISH_SESSION_AUTOSTART_SNIPPET="${FISH_CONFIG_DIR}/conf.d/vmctl-session-autostart.fish"
LEGACY_OMP_THEME_DIR="${TARGET_HOME}/.config/oh-my-posh"
LEGACY_OMP_SNIPPET="${FISH_CONFIG_DIR}/conf.d/oh-my-posh.fish"
ZSHRC_PATH="${TARGET_HOME}/.zshrc"
ZPROFILE_PATH="${TARGET_HOME}/.zprofile"
BASH_PROFILE_PATH="${TARGET_HOME}/.bash_profile"
RUSTUP_HOME="${TARGET_HOME}/.rustup"
CARGO_HOME="${TARGET_HOME}/.cargo"
LOCAL_BIN_DIR="${TARGET_HOME}/.local/bin"
ZEN_INSTALL_DIR="${TARGET_HOME}/.local/opt/zen-browser"
ZEN_APPIMAGE_PATH="${ZEN_INSTALL_DIR}/zen-aarch64.AppImage"
ZEN_EXTRACT_DIR="${ZEN_INSTALL_DIR}/app"
ZEN_WRAPPER_PATH="${LOCAL_BIN_DIR}/zen-browser"
CARGO_INSTALL_TARGET_DIR="${TARGET_HOME}/.cache/cargo-install-target"
ZEN_BROWSER_URL="https://github.com/zen-browser/desktop/releases/latest/download/zen-aarch64.AppImage"
BOOTSTRAP_DNS_SERVERS="1.1.1.1 8.8.8.8"

log() {
  printf '[guest-bootstrap] %s\n' "$*"
}

die() {
  printf '[guest-bootstrap] ERROR: %s\n' "$*" >&2
  exit 1
}

retry() {
  local attempts="$1"
  shift
  local attempt=0
  while [[ "${attempt}" -lt "${attempts}" ]]; do
    attempt=$((attempt + 1))
    if "$@"; then
      return 0
    fi
    if [[ "${attempt}" -lt "${attempts}" ]]; then
      sleep 5
    fi
  done
  return 1
}

as_root() {
  if [[ "$(id -u)" -eq 0 ]]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    die "need root privileges to install packages"
  fi
}

as_target_shell() {
  local cmd="$1"
  if [[ "$(id -un)" == "${TARGET_USER}" ]]; then
    bash -lc "${cmd}"
  elif command -v runuser >/dev/null 2>&1; then
    as_root runuser -u "${TARGET_USER}" -- bash -lc "${cmd}"
  else
    as_root su - "${TARGET_USER}" -s /bin/bash -c "${cmd}"
  fi
}

retry_as_target_shell() {
  local cmd="$1"
  retry 5 as_target_shell "${cmd}"
}

default_brew_packages() {
  cat <<'EOF'
helix
zellij
zig
opencode
lazygit
gitui
EOF
}

default_cargo_packages() {
  cat <<'EOF'
fresh-editor fresh
EOF
}

brew_packages() {
  if [[ -n "${BOOTSTRAP_BREW_PACKAGES}" ]]; then
    printf '%s\n' "${BOOTSTRAP_BREW_PACKAGES}" | tr ' ' '\n' | sed '/^$/d'
    return 0
  fi
  default_brew_packages
}

cargo_packages() {
  if [[ -n "${BOOTSTRAP_CARGO_PACKAGES}" ]]; then
    printf '%s\n' "${BOOTSTRAP_CARGO_PACKAGES}" \
      | tr ',' '\n' \
      | sed 's/:/ /' \
      | sed '/^$/d'
    return 0
  fi
  default_cargo_packages
}

validate_choices() {
  case "${DEFAULT_SHELL}" in
    fish|zsh) ;;
    *) die "unsupported DEFAULT_SHELL: ${DEFAULT_SHELL}" ;;
  esac
  case "${DEFAULT_EDITOR}" in
    neovim|helix) ;;
    *) die "unsupported DEFAULT_EDITOR: ${DEFAULT_EDITOR}" ;;
  esac
  case "${WINDOW_MANAGER}" in
    sway|xfce) ;;
    *) die "unsupported WINDOW_MANAGER: ${WINDOW_MANAGER}" ;;
  esac
}

install_packages() {
  if command -v xbps-install >/dev/null 2>&1; then
    local xbps_args=(-Sy fish-shell zsh curl unzip ca-certificates xz bash git wget file sudo chrony neovim gcc ghostty ghostty-terminfo mesa mesa-dri xorg xfce4 xfce4-terminal fcitx5 fcitx5-chinese-addons fcitx5-configtool fcitx5-gtk+2 fcitx5-gtk+3 fcitx5-gtk4 fcitx5-qt5 fcitx5-qt6 noto-fonts-cjk noto-fonts-emoji)
    if [[ -n "${BOOTSTRAP_XBPS_REPOSITORY}" ]]; then
      xbps_args=(-R "${BOOTSTRAP_XBPS_REPOSITORY}" "${xbps_args[@]}")
    fi
    if ! retry 5 as_root env XBPS_ALLOW_RESTRICTED=yes xbps-install "${xbps_args[@]}"; then
      repair_resolv_conf
      retry 5 as_root env XBPS_ALLOW_RESTRICTED=yes xbps-install "${xbps_args[@]}"
    fi
  elif command -v pacman >/dev/null 2>&1; then
    as_root pacman -Sy --needed --noconfirm fish zsh curl unzip git wget bash file sudo xfce4 xfce4-terminal
  elif command -v apt-get >/dev/null 2>&1; then
    as_root apt-get update
    as_root apt-get install -y fish zsh curl unzip ca-certificates xz-utils git wget bash file sudo xfce4 xfce4-terminal xorg
  else
    die "unsupported package manager"
  fi
}

repair_resolv_conf() {
  local resolv_conf=""
  for ns in ${BOOTSTRAP_DNS_SERVERS}; do
    resolv_conf+="nameserver ${ns}"$'\n'
  done
  printf '%s' "${resolv_conf}" | as_root tee /etc/resolv.conf >/dev/null
}

install_starship() {
  retry_as_target_shell "
    export HOME=${TARGET_HOME@Q}
    eval \"\$(${HOMEBREW_PREFIX@Q}/bin/brew shellenv)\"
    if ! command -v starship >/dev/null 2>&1; then
      brew install starship
    fi
  "
}

install_fnm_and_node() {
  retry_as_target_shell "
    export HOME=${TARGET_HOME@Q}
    eval \"\$(${HOMEBREW_PREFIX@Q}/bin/brew shellenv)\"
    if ! command -v fnm >/dev/null 2>&1; then
      brew install fnm
    fi
    if brew list --versions node >/dev/null 2>&1; then
      brew uninstall --ignore-dependencies node || true
    fi
    latest_lts=\$(fnm list-remote --lts --latest | sed -E 's/^[*[:space:]]+//' | awk 'NF {print \$1}' | tail -n 1)
    [ -n \"\${latest_lts}\" ] || { echo \"[guest-bootstrap] ERROR: unable to resolve latest LTS Node.js\" >&2; exit 1; }
    fnm install --corepack-enabled \"\${latest_lts}\"
    fnm default \"\${latest_lts}\"
  "
}

ensure_default_editor_installed() {
  [[ "${DEFAULT_EDITOR}" == "helix" ]] || return 0

  retry_as_target_shell "
    export HOME=${TARGET_HOME@Q}
    eval \"\$(${HOMEBREW_PREFIX@Q}/bin/brew shellenv)\"
    if ! command -v hx >/dev/null 2>&1; then
      brew install helix
    fi
  "
}

install_rust() {
  retry_as_target_shell "
    export HOME=${TARGET_HOME@Q}
    export CARGO_HOME=${CARGO_HOME@Q}
    export RUSTUP_HOME=${RUSTUP_HOME@Q}
    curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --no-modify-path
    \"${CARGO_HOME}/bin/rustup\" toolchain install stable
    \"${CARGO_HOME}/bin/rustup\" default stable
  "
}

install_homebrew() {
  if [[ -x "${HOMEBREW_PREFIX}/bin/brew" ]]; then
    return 0
  fi

  retry_as_target_shell "
    export HOME=${TARGET_HOME@Q}
    tmp_script=\$(mktemp)
    curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh -o \"\${tmp_script}\"
    NONINTERACTIVE=1 CI=1 /bin/bash \"\${tmp_script}\"
    rm -f \"\${tmp_script}\"
  "
}

install_brew_packages() {
  local packages=()
  local package
  while IFS= read -r package; do
    [[ -n "${package}" ]] || continue
    packages+=("${package}")
  done < <(brew_packages)

  [[ "${#packages[@]}" -gt 0 ]] || return 0

  local brew_list=""
  for package in "${packages[@]}"; do
    brew_list+="${package}"$'\n'
  done

  retry_as_target_shell "
    export HOME=${TARGET_HOME@Q}
    eval \"\$(${HOMEBREW_PREFIX@Q}/bin/brew shellenv)\"
    while IFS= read -r package; do
      [ -n \"\${package}\" ] || continue
      if ! brew install \"\${package}\"; then
        echo \"[guest-bootstrap] WARN: brew install failed for \${package}\" >&2
      fi
    done <<'SHELLEOF'
${brew_list}
SHELLEOF
  "
}

install_cargo_packages() {
  local specs=()
  local spec
  while IFS= read -r spec; do
    [[ -n "${spec}" ]] || continue
    specs+=("${spec}")
  done < <(cargo_packages)

  [[ "${#specs[@]}" -gt 0 ]] || return 0

  local cargo_list=""
  for spec in "${specs[@]}"; do
    cargo_list+="${spec}"$'\n'
  done

  retry_as_target_shell "
    export HOME=${TARGET_HOME@Q}
    export CARGO_HOME=${CARGO_HOME@Q}
    export PATH=${CARGO_HOME@Q}/bin:\$PATH
    export CARGO_TARGET_DIR=${CARGO_INSTALL_TARGET_DIR@Q}
    mkdir -p ${CARGO_INSTALL_TARGET_DIR@Q}
    while IFS= read -r spec; do
      [ -n \"\${spec}\" ] || continue
      crate=\${spec%% *}
      command_name=\${spec#* }
      if [ \"\${command_name}\" = \"\${spec}\" ]; then
        command_name=\${crate}
      fi
      if command -v \"\${command_name}\" >/dev/null 2>&1; then
        continue
      fi
      if cargo install --list | grep -q \"^\${crate} v\"; then
        continue
      fi
      if ! CARGO_BUILD_JOBS=1 cargo install --locked -j 1 \"\${crate}\"; then
        echo \"[guest-bootstrap] WARN: cargo install failed for \${crate}\" >&2
      fi
    done <<'SHELLEOF'
${cargo_list}
SHELLEOF
  "
}

cleanup_legacy_prompt_config() {
  rm -rf "${LEGACY_OMP_THEME_DIR}"
  rm -f "${LEGACY_OMP_SNIPPET}" "${LOCAL_BIN_DIR}/oh-my-posh"
}

write_starship_config() {
  retry_as_target_shell "
    export HOME=${TARGET_HOME@Q}
    mkdir -p \$(dirname ${STARSHIP_CONFIG_PATH@Q})
    curl -fsSL --retry 5 --retry-delay 2 --retry-connrefused ${STARSHIP_PRESET_URL@Q} -o ${STARSHIP_CONFIG_PATH@Q}
  "
}

write_git_config() {
  cat >"${TARGET_HOME}/.gitconfig" <<GITEOF
[core]
	editor = {{.EditorCmd}}
[init]
	defaultBranch = main
[pull]
	rebase = false
[push]
	autoSetupRemote = true
[rebase]
	autoStash = true
[merge]
	conflictstyle = zdiff3
GITEOF

  if [[ -n "${GIT_USER_NAME}" ]] || [[ -n "${GIT_USER_EMAIL}" ]]; then
    {
      printf '\n[user]\n'
      if [[ -n "${GIT_USER_NAME}" ]]; then
        printf '\tname = %s\n' "${GIT_USER_NAME}"
      fi
      if [[ -n "${GIT_USER_EMAIL}" ]]; then
        printf '\temail = %s\n' "${GIT_USER_EMAIL}"
      fi
    } >>"${TARGET_HOME}/.gitconfig"
  fi
}
write_fish_config() {
  mkdir -p "${FISH_CONFIG_DIR}/conf.d"
  cat >"${FISH_CONFIG_SNIPPET}" <<FISHEOF
set -gx COLORTERM truecolor
set -g fish_term24bit 1
set -gx XDG_RUNTIME_DIR \$HOME/.local/run
mkdir -p \$XDG_RUNTIME_DIR
chmod 700 \$XDG_RUNTIME_DIR
set -gx PATH \$HOME/.local/bin \$PATH
set -gx PATH \$HOME/.cargo/bin \$PATH
set -gx EDITOR {{.EditorCmd}}
set -gx VISUAL {{.EditorCmd}}
set -gx GTK_IM_MODULE fcitx
set -gx QT_IM_MODULE fcitx
set -gx SDL_IM_MODULE fcitx
set -gx XMODIFIERS @im=fcitx
if test -x {{.HomebrewPrefix}}/bin/brew
  eval ({{.HomebrewPrefix}}/bin/brew shellenv)
end
if command -q fnm
  fnm env --use-on-cd --shell fish | source
end
set -gx STARSHIP_CONFIG \$HOME/.config/starship.toml
if command -q starship
  starship init fish | source
end
FISHEOF
}

write_fish_autostart() {
  mkdir -p "${FISH_CONFIG_DIR}/conf.d"
  cat >"${FISH_SESSION_AUTOSTART_SNIPPET}" <<'FISHEOF'
if status is-interactive
  if test -z "$WAYLAND_DISPLAY"; and test -z "$DISPLAY"
    if string match -q /dev/tty1 (tty 2>/dev/null)
      exec /usr/local/bin/vmctl-session
    end
  end
end
FISHEOF
}
write_zsh_config() {
  cat >"${ZSHRC_PATH}" <<ZSHEOF
export COLORTERM=truecolor
export XDG_RUNTIME_DIR="\${HOME}/.local/run"
mkdir -p "\${XDG_RUNTIME_DIR}"
chmod 700 "\${XDG_RUNTIME_DIR}"
export PATH="\${HOME}/.local/bin:\${HOME}/.cargo/bin:\${PATH}"
export EDITOR={{.EditorCmd}}
export VISUAL={{.EditorCmd}}
export GTK_IM_MODULE=fcitx
export QT_IM_MODULE=fcitx
export SDL_IM_MODULE=fcitx
export XMODIFIERS=@im=fcitx
if [ -x {{.HomebrewPrefix}}/bin/brew ]; then
  eval "\$({{.HomebrewPrefix}}/bin/brew shellenv)"
fi
if command -v fnm >/dev/null 2>&1; then
  eval "\$(fnm env --use-on-cd --shell zsh)"
fi
export STARSHIP_CONFIG="\${HOME}/.config/starship.toml"
if command -v starship >/dev/null 2>&1; then
  eval "\$(starship init zsh)"
fi
ZSHEOF
}

write_zsh_autostart() {
  cat >"${ZPROFILE_PATH}" <<'ZSHEOF'
export XDG_RUNTIME_DIR="${HOME}/.local/run"
mkdir -p "${XDG_RUNTIME_DIR}"
chmod 700 "${XDG_RUNTIME_DIR}"
if [ -z "${WAYLAND_DISPLAY:-}" ] && [ -z "${DISPLAY:-}" ] && [ "$(tty 2>/dev/null)" = "/dev/tty1" ]; then
  exec /usr/local/bin/vmctl-session
fi
ZSHEOF
}
write_bash_profile() {
  cat >"${BASH_PROFILE_PATH}" <<'BASHEOF'
export XDG_RUNTIME_DIR="${HOME}/.local/run"
mkdir -p "${XDG_RUNTIME_DIR}"
chmod 700 "${XDG_RUNTIME_DIR}"
if [ -z "${WAYLAND_DISPLAY:-}" ] && [ -z "${DISPLAY:-}" ] && [ "$(tty 2>/dev/null)" = "/dev/tty1" ]; then
  exec /usr/local/bin/vmctl-session
fi
BASHEOF
}

write_fcitx_profile() {
  local fcitx_dir="${TARGET_HOME}/.config/fcitx5"
  mkdir -p "${fcitx_dir}"
  tee "${fcitx_dir}/profile" >/dev/null <<'FCITXEOF'
[Groups/0]
Name=Default
Default Layout=us
DefaultIM=pinyin

[Groups/0/Items/0]
Name=keyboard-us
Layout=

[Groups/0/Items/1]
Name=pinyin
Layout=

[GroupOrder]
0=Default
FCITXEOF
}

write_fcitx_config() {
  local fcitx_dir="${TARGET_HOME}/.config/fcitx5"
  mkdir -p "${fcitx_dir}/conf"
  tee "${fcitx_dir}/config" >/dev/null <<'FCITXEOF'
[Hotkey]
EnumerateWithTriggerKeys=True
EnumerateSkipFirst=False
ModifierOnlyKeyTimeout=250

[Hotkey/TriggerKeys]
0=Shift_L

[Hotkey/AltTriggerKeys]
0=Caps_Lock

[Hotkey/EnumerateForwardKeys]
0=Shift_L

[Hotkey/PrevPage]
0=Up

[Hotkey/NextPage]
0=Down

[Hotkey/PrevCandidate]
0=Shift+Tab

[Hotkey/NextCandidate]
0=Tab

[Behavior]
ActiveByDefault=False
resetStateWhenFocusIn=No
ShareInputState=No
PreeditEnabledByDefault=True
ShowInputMethodInformation=True
showInputMethodInformationWhenFocusIn=False
CompactInputMethodInformation=True
ShowFirstInputMethodInformation=True
DefaultPageSize=5
EnabledAddons=
DisabledAddons=
PreloadInputMethod=True
OverrideXkbOption=False
CustomXkbOption=
AllowInputMethodForPassword=False
ShowPreeditForPassword=False
AutoSavePeriod=30
FCITXEOF
}

install_zen_browser() {
  retry_as_target_shell "
    export HOME=${TARGET_HOME@Q}
    install_dir=${ZEN_INSTALL_DIR@Q}
    appimage=${ZEN_APPIMAGE_PATH@Q}
    extract_dir=${ZEN_EXTRACT_DIR@Q}
    mkdir -p \"\${install_dir}\"
    if [ ! -x \"\${extract_dir}/AppRun\" ]; then
      tmp_appimage=\$(mktemp \"\${install_dir}/zen.XXXXXX.AppImage\")
      curl -fL --retry 5 --retry-delay 2 --retry-connrefused ${ZEN_BROWSER_URL@Q} -o \"\${tmp_appimage}\"
      chmod 0755 \"\${tmp_appimage}\"
      rm -rf \"\${extract_dir}\" \"\${install_dir}/squashfs-root\"
      (
        cd \"\${install_dir}\"
        \"\${tmp_appimage}\" --appimage-extract >/dev/null
      )
      mv \"\${install_dir}/squashfs-root\" \"\${extract_dir}\"
      mv \"\${tmp_appimage}\" \"\${appimage}\"
    fi
  "
}

write_session_wrapper() {
  as_root mkdir -p /usr/local/bin
{{if eq .WindowManager "sway"}}
  as_root tee /usr/local/bin/vmctl-session >/dev/null <<'SESSIONEOF'
#!/bin/sh
export XDG_CURRENT_DESKTOP=sway
export XDG_SESSION_TYPE=wayland
export WLR_RENDERER=pixman
export WLR_NO_HARDWARE_CURSORS=1
export GTK_IM_MODULE=fcitx
export QT_IM_MODULE=fcitx
export SDL_IM_MODULE=fcitx
export XMODIFIERS=@im=fcitx
export XDG_RUNTIME_DIR="${HOME}/.local/run"
mkdir -p "${XDG_RUNTIME_DIR}"
chmod 700 "${XDG_RUNTIME_DIR}"
if [ -z "${DBUS_SESSION_BUS_ADDRESS:-}" ]; then
  exec dbus-run-session sh -lc '
    sway &
    sway_pid=$!
    for _ in $(seq 1 100); do
      sock=$(find "${XDG_RUNTIME_DIR}" -maxdepth 1 -type s -name "wayland-*" | head -n 1)
      if [ -n "${sock}" ]; then
        export WAYLAND_DISPLAY=$(basename "${sock}")
        break
      fi
      sleep 0.1
    done
    fcitx5 -d -r >/tmp/fcitx5.log 2>&1 || true
    wait "${sway_pid}"
  '
fi
sway &
sway_pid=$!
for _ in $(seq 1 100); do
  sock=$(find "${XDG_RUNTIME_DIR}" -maxdepth 1 -type s -name "wayland-*" | head -n 1)
  if [ -n "${sock}" ]; then
    export WAYLAND_DISPLAY=$(basename "${sock}")
    break
  fi
  sleep 0.1
done
fcitx5 -d -r >/tmp/fcitx5.log 2>&1 || true
wait "${sway_pid}"
SESSIONEOF
{{else}}
  as_root tee /usr/local/bin/vmctl-session >/dev/null <<'SESSIONEOF'
#!/bin/sh
export XDG_CURRENT_DESKTOP=XFCE
export XDG_SESSION_DESKTOP=xfce
export XDG_SESSION_TYPE=x11
export GTK_IM_MODULE=fcitx
export QT_IM_MODULE=fcitx
export SDL_IM_MODULE=fcitx
export XMODIFIERS=@im=fcitx
export XDG_RUNTIME_DIR="${HOME}/.local/run"
mkdir -p "${XDG_RUNTIME_DIR}"
chmod 700 "${XDG_RUNTIME_DIR}"
if [ -z "${DBUS_SESSION_BUS_ADDRESS:-}" ]; then
  exec dbus-run-session startxfce4
fi
exec startxfce4
SESSIONEOF
{{end}}
  as_root chmod 0755 /usr/local/bin/vmctl-session
}

write_chromium_wrapper() {
  as_root mkdir -p /usr/local/bin
  as_root tee /usr/local/bin/vmctl-chromium >/dev/null <<'CHROMEOF'
#!/bin/sh
export GTK_IM_MODULE=fcitx
export XMODIFIERS=@im=fcitx
exec /usr/bin/chromium --ozone-platform=x11 "$@"
CHROMEOF
  as_root chmod 0755 /usr/local/bin/vmctl-chromium

  local app_dir="${TARGET_HOME}/.local/share/applications"
  mkdir -p "${app_dir}"
  tee "${app_dir}/chromium.desktop" >/dev/null <<'CHROMEDESKEOF'
[Desktop Entry]
Version=1.0
Name=Chromium
GenericName=Web Browser
Comment=Access the Internet
Exec=/usr/local/bin/vmctl-chromium %U
StartupNotify=true
Terminal=false
Icon=chromium
Type=Application
Categories=Network;WebBrowser;
MimeType=application/pdf;application/rdf+xml;application/rss+xml;application/xhtml+xml;application/xhtml_xml;application/xml;image/gif;image/jpeg;image/png;image/webp;text/html;text/xml;x-scheme-handler/http;x-scheme-handler/https;x-scheme-handler/chromium;
Actions=new-window;new-private-window;

[Desktop Action new-window]
Name=New Window
Exec=/usr/local/bin/vmctl-chromium

[Desktop Action new-private-window]
Name=New Incognito Window
Exec=/usr/local/bin/vmctl-chromium --incognito
CHROMEDESKEOF
}

write_zen_wrapper() {
  mkdir -p "${LOCAL_BIN_DIR}"
  tee "${ZEN_WRAPPER_PATH}" >/dev/null <<ZENEOF
#!/bin/sh
export GTK_IM_MODULE=fcitx
export QT_IM_MODULE=fcitx
export SDL_IM_MODULE=fcitx
export XMODIFIERS=@im=fcitx
export MOZ_ENABLE_WAYLAND=1
exec ${ZEN_EXTRACT_DIR}/AppRun "\$@"
ZENEOF
  chmod 0755 "${ZEN_WRAPPER_PATH}"

  local app_dir="${TARGET_HOME}/.local/share/applications"
  mkdir -p "${app_dir}"
  tee "${app_dir}/zen-browser.desktop" >/dev/null <<ZENDESKEOF
[Desktop Entry]
Version=1.0
Name=Zen Browser
GenericName=Web Browser
Comment=Browse the web with Zen Browser
Exec=${ZEN_WRAPPER_PATH} %U
StartupNotify=true
Terminal=false
Icon=${ZEN_EXTRACT_DIR}/.DirIcon
Type=Application
Categories=Network;WebBrowser;
MimeType=text/html;text/xml;application/xhtml+xml;x-scheme-handler/http;x-scheme-handler/https;
StartupWMClass=zen
ZENDESKEOF
}

write_swaybar_status() {
  as_root mkdir -p /usr/local/bin
  as_root tee /usr/local/bin/vmctl-swaybar-status >/dev/null <<'BAREOF'
#!/bin/sh
printf '{"version":1}\n[\n[]\n'
while :; do
  im_name="$(fcitx5-remote -n 2>/dev/null || true)"
  case "${im_name}" in
    pinyin) im_label="中" ;;
    keyboard-us|"") im_label="EN" ;;
    *) im_label="${im_name}" ;;
  esac
  time_text="$(date '+%Y-%m-%d %H:%M:%S')"
  printf ',[{"name":"input","full_text":"IM: %s"},{"name":"time","full_text":"%s"}]\n' "${im_label}" "${time_text}"
  sleep 1
done
BAREOF
  as_root chmod 0755 /usr/local/bin/vmctl-swaybar-status
}

configure_timezone() {
  if [[ -n "${BOOTSTRAP_TIMEZONE}" ]] && [[ -e "/usr/share/zoneinfo/${BOOTSTRAP_TIMEZONE}" ]]; then
    as_root ln -snf "/usr/share/zoneinfo/${BOOTSTRAP_TIMEZONE}" /etc/localtime
    printf '%s\n' "${BOOTSTRAP_TIMEZONE}" | as_root tee /etc/timezone >/dev/null
  fi
}

configure_time_sync() {
  if command -v chronyd >/dev/null 2>&1 && [[ -d /etc/sv/chronyd ]]; then
    as_root ln -snsf /etc/sv/chronyd /var/service/chronyd
    retry 5 as_root sv restart chronyd || retry 5 as_root sv start chronyd
  fi
}
{{if eq .WindowManager "sway"}}
write_window_manager_config() {
  as_root mkdir -p /etc/sway/config.d
  as_root tee /etc/sway/config.d/10-vmctl.conf >/dev/null <<'SWAYEOF'
set $term ghostty
unbindsym $mod+Return
bindsym $mod+Return exec $term
set $menu wofi --show drun
unbindsym $mod+d
bindsym $mod+d exec $menu
input type:pointer {
    natural_scroll enabled
}
input type:touchpad {
    natural_scroll enabled
}
SWAYEOF
  as_root tee /etc/sway/config.d/20-vmctl-bar.conf >/dev/null <<'SWAYBAREOF'
bar bar-0 {
    tray_output *
    status_command /usr/local/bin/vmctl-swaybar-status
}
SWAYBAREOF
}
{{end}}

fix_ownership() {
  if [[ "$(id -un)" != "${TARGET_USER}" ]]; then
    as_root chown -R "${TARGET_USER}:$(id -gn "${TARGET_USER}")" \
      "${TARGET_HOME}/.local" \
      "${TARGET_HOME}/.cargo" \
      "${TARGET_HOME}/.rustup" \
      "${FISH_CONFIG_DIR}"
    if [[ -e "${STARSHIP_CONFIG_PATH}" ]]; then
      as_root chown "${TARGET_USER}:$(id -gn "${TARGET_USER}")" "${STARSHIP_CONFIG_PATH}"
    fi
    for path in "${TARGET_HOME}/.gitconfig" "${ZSHRC_PATH}" "${ZPROFILE_PATH}" "${BASH_PROFILE_PATH}"; do
      if [[ -e "${path}" ]]; then
        as_root chown "${TARGET_USER}:$(id -gn "${TARGET_USER}")" "${path}"
      fi
    done
  fi
}

set_default_shell() {
{{if .SetDefaultShell}}
  local shell_path=""
  case "${DEFAULT_SHELL}" in
    fish) shell_path="$(command -v fish)" ;;
    zsh) shell_path="$(command -v zsh)" ;;
  esac
  [[ -n "${shell_path}" ]] || die "${DEFAULT_SHELL} not found after install"

  if [[ "$(getent passwd "${TARGET_USER}" 2>/dev/null || grep "^${TARGET_USER}:" /etc/passwd)" != *"${shell_path}" ]]; then
    as_root chsh -s "${shell_path}" "${TARGET_USER}"
  fi
{{end}}
}

main() {
  validate_choices
  install_packages
  install_rust
  install_homebrew
  install_starship
  install_fnm_and_node
  install_brew_packages
  ensure_default_editor_installed
  install_cargo_packages
  install_zen_browser
  cleanup_legacy_prompt_config
  write_starship_config
  write_git_config
  write_fish_config
  write_fish_autostart
  write_zsh_config
  write_zsh_autostart
  write_bash_profile
  write_fcitx_config
  write_fcitx_profile
  write_session_wrapper
  write_chromium_wrapper
  write_zen_wrapper
  write_swaybar_status
  configure_timezone
  configure_time_sync
{{if eq .WindowManager "sway"}}
  write_window_manager_config
{{end}}
  fix_ownership
  set_default_shell
  mkdir -p ${TARGET_HOME}/repos ${TARGET_HOME}/projects
{{if .ExtraCommands}}
  log "running extra bootstrap commands..."
  bash -lc "{{.ExtraCommands}}" || true
{{end}}
  log "configured ${DEFAULT_SHELL}, ${DEFAULT_EDITOR}, ${WINDOW_MANAGER}, fnm, Starship, Rust, Homebrew tools, Cargo tools, Ghostty, Zen Browser, Chromium, Fcitx5 Chinese input, timezone, and time sync for ${TARGET_USER}"
}

main "$@"
`
