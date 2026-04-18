#!/usr/bin/env bash

set -euo pipefail

TARGET_USER="${TARGET_USER:-$(id -un)}"
TARGET_HOME="${TARGET_HOME:-$(eval echo "~${TARGET_USER}")}"
OMP_THEME_URL="${OMP_THEME_URL:-https://raw.githubusercontent.com/JanDeDobbeleer/oh-my-posh/main/themes/unicorn.omp.json}"
OMP_THEME_NAME="${OMP_THEME_NAME:-unicorn.omp.json}"
SET_DEFAULT_SHELL="${SET_DEFAULT_SHELL:-1}"
HOMEBREW_PREFIX="${HOMEBREW_PREFIX:-/home/linuxbrew/.linuxbrew}"
BOOTSTRAP_DNS_SERVERS="${BOOTSTRAP_DNS_SERVERS:-1.1.1.1 8.8.8.8}"
BOOTSTRAP_XBPS_REPOSITORY="${BOOTSTRAP_XBPS_REPOSITORY:-}"
BOOTSTRAP_TIMEZONE="${BOOTSTRAP_TIMEZONE:-Australia/Sydney}"
ZEN_BROWSER_URL="${ZEN_BROWSER_URL:-https://github.com/zen-browser/desktop/releases/latest/download/zen-aarch64.AppImage}"
BOOTSTRAP_BREW_PACKAGES="${BOOTSTRAP_BREW_PACKAGES:-}"
BOOTSTRAP_CARGO_PACKAGES="${BOOTSTRAP_CARGO_PACKAGES:-}"
GIT_USER_NAME="${GIT_USER_NAME:-}"
GIT_USER_EMAIL="${GIT_USER_EMAIL:-}"

OMP_THEME_DIR="${TARGET_HOME}/.config/oh-my-posh"
OMP_THEME_PATH="${OMP_THEME_DIR}/${OMP_THEME_NAME}"
FISH_CONFIG_DIR="${TARGET_HOME}/.config/fish"
FISH_CONFIG_SNIPPET="${FISH_CONFIG_DIR}/conf.d/oh-my-posh.fish"
FISH_SWAY_AUTOSTART_SNIPPET="${FISH_CONFIG_DIR}/conf.d/vmctl-sway-autostart.fish"
RUSTUP_HOME="${TARGET_HOME}/.rustup"
CARGO_HOME="${TARGET_HOME}/.cargo"
LOCAL_BIN_DIR="${TARGET_HOME}/.local/bin"
ZEN_INSTALL_DIR="${TARGET_HOME}/.local/opt/zen-browser"
ZEN_APPIMAGE_PATH="${ZEN_INSTALL_DIR}/zen-aarch64.AppImage"
ZEN_EXTRACT_DIR="${ZEN_INSTALL_DIR}/app"
ZEN_WRAPPER_PATH="${LOCAL_BIN_DIR}/zen-browser"
CARGO_INSTALL_TARGET_DIR="${TARGET_HOME}/.cache/cargo-install-target"

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

install_packages() {
  if command -v xbps-install >/dev/null 2>&1; then
    local xbps_args=(-Sy fish-shell curl unzip ca-certificates xz bash git wget file sudo chrony neovim ghostty ghostty-terminfo mesa mesa-dri fcitx5 fcitx5-chinese-addons fcitx5-configtool fcitx5-gtk+2 fcitx5-gtk+3 fcitx5-gtk4 fcitx5-qt5 fcitx5-qt6 noto-fonts-cjk noto-fonts-emoji)
    if [[ -n "${BOOTSTRAP_XBPS_REPOSITORY}" ]]; then
      xbps_args=(-R "${BOOTSTRAP_XBPS_REPOSITORY}" "${xbps_args[@]}")
    fi
    if ! retry 5 as_root env XBPS_ALLOW_RESTRICTED=yes xbps-install "${xbps_args[@]}"; then
      repair_resolv_conf
      retry 5 as_root env XBPS_ALLOW_RESTRICTED=yes xbps-install "${xbps_args[@]}"
    fi
  elif command -v pacman >/dev/null 2>&1; then
    as_root pacman -Sy --needed --noconfirm fish curl unzip git wget bash file sudo
  elif command -v apt-get >/dev/null 2>&1; then
    as_root apt-get update
    as_root apt-get install -y fish curl unzip ca-certificates xz-utils git wget bash file sudo
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

install_oh_my_posh() {
  retry_as_target_shell "
    export HOME=${TARGET_HOME@Q}
    mkdir -p ${LOCAL_BIN_DIR@Q}
    arch=\$(uname -m | tr '[:upper:]' '[:lower:]')
    case \"\${arch}\" in
      x86_64) arch=amd64 ;;
      arm64|aarch64) arch=arm64 ;;
      armv*) arch=arm ;;
    esac
    curl -fsSL --retry 5 --retry-delay 2 --retry-connrefused \
      \"https://cdn.ohmyposh.dev/releases/latest/posh-linux-\${arch}\" \
      -o ${LOCAL_BIN_DIR@Q}/oh-my-posh
    chmod 0755 ${LOCAL_BIN_DIR@Q}/oh-my-posh
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
    done <<'EOF'
${brew_list}
EOF
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
    done <<'EOF'
${cargo_list}
EOF
  "
}

write_theme() {
  mkdir -p "${OMP_THEME_DIR}"
  retry 5 curl -fsSL --retry 5 --retry-delay 2 --retry-connrefused "${OMP_THEME_URL}" -o "${OMP_THEME_PATH}"
}

write_git_config() {
  cat >"${TARGET_HOME}/.gitconfig" <<EOF
[core]
	editor = nvim
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
EOF

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
  cat >"${FISH_CONFIG_SNIPPET}" <<EOF
set -gx COLORTERM truecolor
set -g fish_term24bit 1
set -gx XDG_RUNTIME_DIR \$HOME/.local/run
mkdir -p \$XDG_RUNTIME_DIR
chmod 700 \$XDG_RUNTIME_DIR
set -gx PATH \$HOME/.local/bin \$PATH
set -gx PATH \$HOME/.cargo/bin \$PATH
set -gx GTK_IM_MODULE fcitx
set -gx QT_IM_MODULE fcitx
set -gx SDL_IM_MODULE fcitx
set -gx XMODIFIERS @im=fcitx
if test -x ${HOMEBREW_PREFIX}/bin/brew
  eval (${HOMEBREW_PREFIX}/bin/brew shellenv)
end
oh-my-posh init fish --config ${OMP_THEME_PATH} | source
EOF
}

write_sway_autostart() {
  mkdir -p "${FISH_CONFIG_DIR}/conf.d"
  cat >"${FISH_SWAY_AUTOSTART_SNIPPET}" <<'EOF'
if status is-interactive
  if test -z "$WAYLAND_DISPLAY"; and test -z "$DISPLAY"
    set current_tty (tty 2>/dev/null)
    if test "$current_tty" = "/dev/tty1"
      exec /usr/local/bin/vmctl-sway-session
    end
  end
end
EOF
}

write_fcitx_profile() {
  local fcitx_dir="${TARGET_HOME}/.config/fcitx5"
  mkdir -p "${fcitx_dir}"
  tee "${fcitx_dir}/profile" >/dev/null <<'EOF'
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
EOF
}

write_fcitx_config() {
  local fcitx_dir="${TARGET_HOME}/.config/fcitx5"
  mkdir -p "${fcitx_dir}/conf"
  tee "${fcitx_dir}/config" >/dev/null <<'EOF'
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
EOF
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

write_sway_session_wrapper() {
  as_root mkdir -p /usr/local/bin
  as_root tee /usr/local/bin/vmctl-sway-session >/dev/null <<'EOF'
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
EOF
  as_root chmod 0755 /usr/local/bin/vmctl-sway-session
}

write_chromium_wrapper() {
  as_root mkdir -p /usr/local/bin
  as_root tee /usr/local/bin/vmctl-chromium >/dev/null <<'EOF'
#!/bin/sh
export GTK_IM_MODULE=fcitx
export XMODIFIERS=@im=fcitx
exec /usr/bin/chromium --ozone-platform=x11 "$@"
EOF
  as_root chmod 0755 /usr/local/bin/vmctl-chromium

  local app_dir="${TARGET_HOME}/.local/share/applications"
  mkdir -p "${app_dir}"
  tee "${app_dir}/chromium.desktop" >/dev/null <<'EOF'
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
EOF
}

write_zen_wrapper() {
  mkdir -p "${LOCAL_BIN_DIR}"
  tee "${ZEN_WRAPPER_PATH}" >/dev/null <<EOF
#!/bin/sh
export GTK_IM_MODULE=fcitx
export QT_IM_MODULE=fcitx
export SDL_IM_MODULE=fcitx
export XMODIFIERS=@im=fcitx
export MOZ_ENABLE_WAYLAND=1
exec ${ZEN_EXTRACT_DIR}/AppRun "\$@"
EOF
  chmod 0755 "${ZEN_WRAPPER_PATH}"

  local app_dir="${TARGET_HOME}/.local/share/applications"
  mkdir -p "${app_dir}"
  tee "${app_dir}/zen-browser.desktop" >/dev/null <<EOF
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
EOF
}

write_swaybar_status() {
  as_root mkdir -p /usr/local/bin
  as_root tee /usr/local/bin/vmctl-swaybar-status >/dev/null <<'EOF'
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
EOF
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

write_sway_config_override() {
  as_root mkdir -p /etc/sway/config.d
as_root tee /etc/sway/config.d/10-vmctl.conf >/dev/null <<'EOF'
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
EOF
as_root tee /etc/sway/config.d/20-vmctl-bar.conf >/dev/null <<'EOF'
bar bar-0 {
    tray_output *
    status_command /usr/local/bin/vmctl-swaybar-status
}
EOF
}

fix_ownership() {
  if [[ "$(id -un)" != "${TARGET_USER}" ]]; then
    as_root chown -R "${TARGET_USER}:$(id -gn "${TARGET_USER}")" \
      "${TARGET_HOME}/.local" \
      "${TARGET_HOME}/.cargo" \
      "${TARGET_HOME}/.rustup" \
      "${TARGET_HOME}/.config/oh-my-posh" \
      "${FISH_CONFIG_DIR}"
  fi
}

set_default_shell() {
  [[ "${SET_DEFAULT_SHELL}" == "1" ]] || return 0

  local fish_path
  fish_path="$(command -v fish)"
  [[ -n "${fish_path}" ]] || die "fish not found after install"

  if [[ "$(getent passwd "${TARGET_USER}" 2>/dev/null || grep "^${TARGET_USER}:" /etc/passwd)" != *"${fish_path}" ]]; then
    as_root chsh -s "${fish_path}" "${TARGET_USER}"
  fi
}

main() {
  install_packages
  install_oh_my_posh
  install_rust
  install_homebrew
  install_brew_packages
  install_cargo_packages
  install_zen_browser
  write_theme
  write_git_config
  write_fish_config
  write_fcitx_config
  write_fcitx_profile
  write_sway_session_wrapper
  write_chromium_wrapper
  write_zen_wrapper
  write_swaybar_status
  write_sway_autostart
  configure_timezone
  configure_time_sync
  write_sway_config_override
  fix_ownership
  set_default_shell
  log "configured git, fish, oh-my-posh (${OMP_THEME_NAME}), Rust, Homebrew tools, Cargo tools, Ghostty, Zen Browser, Neovim, Fcitx5 Chinese input, timezone, and time sync for ${TARGET_USER}"
}

main "$@"
