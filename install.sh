#!/usr/bin/env bash
set -euo pipefail

# Sonar installer
# Usage: curl -fsSL <url>/install.sh | bash

INSTALL_DIR="${SONAR_INSTALL_DIR:-$HOME/.sonar/bin}"
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors (respect NO_COLOR)
if [ -z "${NO_COLOR:-}" ] && [ -t 1 ]; then
    BOLD='\033[1m'
    CYAN='\033[36m'
    GREEN='\033[32m'
    DIM='\033[2m'
    RESET='\033[0m'
else
    BOLD='' CYAN='' GREEN='' DIM='' RESET=''
fi

info() { printf "${BOLD}${CYAN}sonar${RESET} %s\n" "$1"; }
success() { printf "${GREEN}✓${RESET} %s\n" "$1"; }
dim() { printf "${DIM}%s${RESET}\n" "$1"; }

# Build
info "Building sonar..."
if ! command -v go &>/dev/null; then
    echo "Error: go is not installed. Install it from https://go.dev/dl/" >&2
    exit 1
fi

info "$(pwd)"
(cd "${REPO_DIR}/cli" && go build -o sonar .)

# Install
info "Installing to $INSTALL_DIR"
mkdir -p "$INSTALL_DIR"
cp "$REPO_DIR/sonar" "$INSTALL_DIR/sonar"
chmod +x "$INSTALL_DIR/sonar"
success "Installed sonar to $INSTALL_DIR/sonar"

# Add to PATH if not already there
add_to_path() {
    local shell_config="$1"
    local label="$2"

    if [ ! -f "$shell_config" ]; then
        return 1
    fi

    if grep -q "$INSTALL_DIR" "$shell_config" 2>/dev/null; then
        dim "PATH already configured in $label"
        return 0
    fi

    printf '\n# sonar\nexport PATH="%s:$PATH"\n' "$INSTALL_DIR" >> "$shell_config"
    success "Added sonar to PATH in $label"
    return 0
}

if echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
    dim "sonar is already in PATH"
else
    modified=false
    current_shell="$(basename "${SHELL:-bash}")"

    case "$current_shell" in
        zsh)
            add_to_path "$HOME/.zshrc" "~/.zshrc" && modified=true
            ;;
        bash)
            # Prefer .bashrc, fall back to .bash_profile on macOS
            if [ -f "$HOME/.bashrc" ]; then
                add_to_path "$HOME/.bashrc" "~/.bashrc" && modified=true
            elif [ -f "$HOME/.bash_profile" ]; then
                add_to_path "$HOME/.bash_profile" "~/.bash_profile" && modified=true
            fi
            ;;
    esac

    if [ "$modified" = true ]; then
        echo ""
        info "Restart your terminal or run:"
        dim "  source ~/.${current_shell}rc"
    fi
fi

echo ""
success "Done! Run 'sonar' to get started."
