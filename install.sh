#!/bin/bash

set -e

# anytype-cli installation script
# Usage: /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/anyproto/anytype-cli/HEAD/install.sh)"

REPO_OWNER="anyproto"
REPO_NAME="anytype-cli"
BINARY_NAME="anytype"
INSTALL_DIR="${HOME}/.local/bin"
GITHUB_API="https://api.github.com"
GITHUB_DOWNLOAD="https://github.com"
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

error() {
    echo -e "${RED}Error: $1${NC}" >&2
    exit 1
}

success() {
    echo -e "${GREEN}✓ $1${NC}" >&2
}

info() {
    echo -e "${BLUE}→ $1${NC}" >&2
}

warning() {
    echo -e "${YELLOW}⚠ $1${NC}" >&2
}

detect_platform() {
    local os arch
    case "$(uname -s)" in
        Linux*)     os="linux";;
        Darwin*)    os="darwin";;
        CYGWIN*|MINGW*|MSYS*) os="windows";;
        *)          error "Unsupported operating system: $(uname -s)";;
    esac
    case "$(uname -m)" in
        x86_64|amd64)   arch="amd64";;
        aarch64|arm64)  arch="arm64";;
        armv7l|armv7)   arch="arm";;
        i386|i686)      arch="386";;
        *)              error "Unsupported architecture: $(uname -m)";;
    esac

    echo "${os}-${arch}"
}

check_requirements() {
    local missing_deps=()
    
    for cmd in curl tar; do
        if ! command -v "$cmd" &> /dev/null; then
            missing_deps+=("$cmd")
        fi
    done
    
    if [ ${#missing_deps[@]} -ne 0 ]; then
        error "Missing required dependencies: ${missing_deps[*]}\nPlease install them and try again."
    fi
}

get_latest_version() {
    local api_url="${GITHUB_API}/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest"
    local version
    
    info "Fetching latest release information..."
    
    if [ -n "$GITHUB_TOKEN" ]; then
        version=$(curl -sL -H "Authorization: token $GITHUB_TOKEN" "$api_url" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    else
        version=$(curl -sL "$api_url" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    fi
    
    if [ -z "$version" ]; then
        if [ -n "$GITHUB_TOKEN" ]; then
            error "Failed to fetch the latest release version. Check your GITHUB_TOKEN and repository access."
        else
            error "Failed to fetch the latest release version. If this is a private repository, set GITHUB_TOKEN environment variable."
        fi
    fi
    
    echo "$version"
}

download_binary() {
    local version="$1"
    local platform="$2"
    local temp_dir="$3"
    local filename="anytype-cli-${version}-${platform}.tar.gz"
    
    info "Downloading ${BINARY_NAME} ${version} for ${platform}..."
    
    if [ -n "$GITHUB_TOKEN" ]; then
        local api_url="${GITHUB_API}/repos/${REPO_OWNER}/${REPO_NAME}/releases/tags/${version}"
        local release_json
        
        release_json=$(curl -sL -H "Authorization: token $GITHUB_TOKEN" "$api_url")
        
        if echo "$release_json" | grep -q '"message"'; then
            local error_msg=$(echo "$release_json" | grep '"message"' | sed -E 's/.*"message": "([^"]+)".*/\1/')
            error "GitHub API error: $error_msg"
        fi
        
        local asset_url
        asset_url=$(echo "$release_json" | \
            grep -B3 "\"name\": \"$filename\"" | \
            grep '"url"' | \
            tail -1 | \
            sed -E 's/.*"url": "([^"]+)".*/\1/')
        
        if [ -z "$asset_url" ]; then
            error "Failed to find release asset: $filename"
        fi
        
        if ! curl -fL --progress-bar -H "Authorization: token $GITHUB_TOKEN" -H "Accept: application/octet-stream" "$asset_url" -o "$temp_dir/${BINARY_NAME}.tar.gz"; then
            error "Failed to download binary. Check your GITHUB_TOKEN and repository access."
        fi
    else
        local download_url="${GITHUB_DOWNLOAD}/${REPO_OWNER}/${REPO_NAME}/releases/download/${version}/${filename}"
        if ! curl -fL --progress-bar "$download_url" -o "$temp_dir/${BINARY_NAME}.tar.gz"; then
            error "Failed to download binary from: $download_url. If this is a private repository, set GITHUB_TOKEN environment variable."
        fi
    fi
    
    info "Extracting binary..."
    if ! tar -xzf "$temp_dir/${BINARY_NAME}.tar.gz" -C "$temp_dir"; then
        error "Failed to extract the downloaded archive"
    fi
    
    local binary_path
    if [[ "$platform" == windows* ]]; then
        binary_path=$(find "$temp_dir" -type f -name "${BINARY_NAME}.exe" | head -n1)
    else
        binary_path=$(find "$temp_dir" -type f -name "$BINARY_NAME" | head -n1)
    fi
    
    if [ -z "$binary_path" ] || [ ! -f "$binary_path" ]; then
        error "Binary not found in the extracted archive"
    fi
    
    echo "$binary_path"
}

install_binary() {
    local binary_path="$1"
    if [ ! -d "$INSTALL_DIR" ]; then
        info "Creating installation directory: $INSTALL_DIR"
        mkdir -p "$INSTALL_DIR"
    fi
    if [ -f "$INSTALL_DIR/$BINARY_NAME" ]; then
        warning "Existing installation found at $INSTALL_DIR/$BINARY_NAME"
        read -p "Do you want to overwrite it? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            info "Installation cancelled"
            exit 0
        fi
    fi
    
    info "Installing binary to $INSTALL_DIR/$BINARY_NAME"
    
    if ! cp "$binary_path" "$INSTALL_DIR/$BINARY_NAME"; then
        error "Failed to copy binary to installation directory"
    fi
    
    if ! chmod +x "$INSTALL_DIR/$BINARY_NAME"; then
        error "Failed to make binary executable"
    fi
}

check_path() {
    if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
        warning "$INSTALL_DIR is not in your PATH"
        local shell_rc=""
        local shell_name=""
        
        if [ -n "$BASH_VERSION" ]; then
            shell_rc="$HOME/.bashrc"
            shell_name="bash"
        elif [ -n "$ZSH_VERSION" ]; then
            shell_rc="$HOME/.zshrc"
            shell_name="zsh"
        else
            shell_rc="$HOME/.profile"
            shell_name="shell"
        fi
        
        echo
        info "To add it to your PATH, run:"
        echo "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> $shell_rc"
        echo "  source $shell_rc"
        echo
        info "Or for this session only:"
        echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
    fi
}

verify_installation() {
    if command -v "$BINARY_NAME" &> /dev/null; then
        success "${BINARY_NAME} installed successfully!"
    else
        echo
        info "Run the following to use ${BINARY_NAME}:"
        echo "  $INSTALL_DIR/$BINARY_NAME"
    fi
}

main() {
    local temp_dir
    temp_dir=$(mktemp -d)
    trap "rm -rf '$temp_dir'" EXIT

    echo "---------------------"
    echo "anytype-cli installer"
    echo "---------------------"
    echo
    check_requirements
    local platform
    platform=$(detect_platform)
    success "Detected platform: $platform"
    local version
    version=$(get_latest_version)
    success "Latest version: $version"
    echo
    local binary_path
    binary_path=$(download_binary "$version" "$platform" "$temp_dir")
    install_binary "$binary_path"
    check_path
    verify_installation
    echo
    info "Get started with: ${BINARY_NAME} --help"
    info "Learn more: https://github.com/${REPO_OWNER}/${REPO_NAME}"
}

main "$@"
