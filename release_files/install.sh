#!/bin/sh
# This code is based on the netbird-installer contribution by physk on GitHub.
# Source: https://github.com/physk/netbird-installer
set -e

OWNER="netbirdio"
REPO="netbird"
CLI_APP="netbird"
UI_APP="netbird-ui"

# Set default variable
OS_NAME=""
OS_TYPE=""
ARCH="$(uname -m)"
PACKAGE_MANAGER=""
INSTALL_DIR=""

get_latest_release() {
    curl -s "https://api.github.com/repos/${OWNER}/${REPO}/releases/latest" \
    | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/'
}

download_release_binary() {
    VERSION=$(get_latest_release)
    BASE_URL="https://github.com/${OWNER}/${REPO}/releases/download"
    BINARY_BASE_NAME="${VERSION#v}_${OS_TYPE}_${ARCH}.tar.gz"
    
    # for Darwin, download the signed Netbird-UI
    if [ "$OS_TYPE" = "darwin" ] && [ "$1" = "$UI_APP" ]; then
        BINARY_BASE_NAME="${VERSION#v}_${OS_TYPE}_${ARCH}_signed.zip"
    fi

    BINARY_NAME="$1_${BINARY_BASE_NAME}"
    DOWNLOAD_URL="${BASE_URL}/${VERSION}/${BINARY_NAME}"

    echo "Installing $1 from $DOWNLOAD_URL"
    cd /tmp && curl -LO "$DOWNLOAD_URL" 
    
    if [ "$OS_TYPE" = "darwin" ] && [ "$1" = "$UI_APP" ]; then
        INSTALL_DIR="/Applications/NetBird UI.app"
        
        # Unzip the app and move to INSTALL_DIR
        unzip -q -o "$BINARY_NAME"
        mv "netbird_ui_${OS_TYPE}_${ARCH}" "$INSTALL_DIR"
    else
        tar -xzvf "$BINARY_NAME"
        sudo mv "${1%_"${BINARY_BASE_NAME}"}" "$INSTALL_DIR"
    fi
}

add_apt_repo() {
    sudo apt-get update
    sudo apt-get install ca-certificates gnupg -y
        
    curl -sSL https://pkgs.wiretrustee.com/debian/public.key \
    | sudo gpg --dearmor --output /usr/share/keyrings/wiretrustee-archive-keyring.gpg

    APT_REPO="deb [signed-by=/usr/share/keyrings/wiretrustee-archive-keyring.gpg] https://pkgs.wiretrustee.com/debian stable main"
    echo "$APT_REPO" | sudo tee /etc/apt/sources.list.d/wiretrustee.list

    sudo apt-get update
}

add_rpm_repo() {
cat <<-EOF | sudo tee /etc/yum.repos.d/netbird.repo
[Netbird]
name=Netbird
baseurl=https://pkgs.netbird.io/yum/
enabled=1
gpgcheck=0
gpgkey=https://pkgs.netbird.io/yum/repodata/repomd.xml.key
repo_gpgcheck=1
EOF
} 

add_aur_repo() {
    INSTALL_PKGS="git base-devel go"
    REMOVE_PKGS=""

    # Check if dependencies are installed
    for PKG in $INSTALL_PKGS; do
        if ! pacman -Q "$PKG" > /dev/null 2>&1; then
            # Install missing package(s)
            sudo pacman -S "$PKG" --noconfirm

            # Add installed package for clean up later
            REMOVE_PKGS="$REMOVE_PKGS $PKG"
        fi
    done

    # Build package from AUR
    cd /tmp && git clone https://aur.archlinux.org/netbird.git 
    cd netbird && makepkg -sri --noconfirm

    if ! $SKIP_UI_APP; then 
        cd /tmp && git clone https://aur.archlinux.org/netbird-ui.git
        cd netbird-ui && makepkg -sri --noconfirm
    fi

    # Clean up the installed packages
    sudo pacman -Rs "$REMOVE_PKGS" --noconfirm
}

install_native_binaries() {
    # Checks  for supported architecture
    case "$ARCH" in
        x86_64|amd64)
            ARCH="amd64"
        ;;
        i?86|x86)
            ARCH="386"
        ;;
        aarch64|arm64)
            ARCH="arm64"
        ;;
        *)
            echo "Architecture ${ARCH} not supported"
            exit 2
        ;;
    esac

    # download and copy binaries to INSTALL_DIR
    download_release_binary "$CLI_APP"
    if ! $SKIP_UI_APP; then 
        download_release_binary "$UI_APP"
    fi  
}

install_netbird() {
    # Check if netbird CLI is installed
    if [ -x "$(command -v netbird)" ]; then
        if  netbird status > /dev/null 2>&1; then
            echo "Netbird service is running, please stop it before proceeding"
        fi

        echo "Netbird seems to be installed already, please remove it before proceeding"
        exit 1
    fi

    # Checks if SKIP_UI_APP env is set
    if [ -z "$SKIP_UI_APP" ]; then
        SKIP_UI_APP=false
    else
        if $SKIP_UI_APP; then 
            echo "SKIP_UI_APP has been set to true in the environment"
            echo "Netbird UI installation will be omitted based on your preference"
        fi
    fi

    # Identify OS name and default package manager
    if type uname >/dev/null 2>&1; then
	case "$(uname)" in
        Linux)
            OS_NAME="$(. /etc/os-release && echo "$ID")" 
            OS_TYPE="linux"
            INSTALL_DIR="/usr/bin"
            
            # Allow netbird UI installation for x64 arch only
            if [ "$ARCH" != "amd64" ] && [ "$ARCH" != "arm64" ] \
                && [ "$ARCH" != "x86_64" ];then
                SKIP_UI_APP=true
                echo "Netbird UI installation will be omitted as $ARCH is not a compactible architecture"
            fi

            # Allow netbird UI installation for linux running desktop enviroment
            if [ -z "$XDG_CURRENT_DESKTOP" ];then
                    SKIP_UI_APP=true
                    echo "Netbird UI installation will be omitted as Linux does not run desktop environment"
            fi

            # Check the availability of a compactible package manager
            if [ -x "$(command -v apt)" ]; then
                PACKAGE_MANAGER="apt"
                echo "The installation will be performed using apt package manager"
            elif [ -x "$(command -v dnf)" ]; then
                PACKAGE_MANAGER="dnf"
                echo "The installation will be performed using dnf package manager"
            elif [ -x "$(command -v yum)" ]; then
                PACKAGE_MANAGER="yum"
                echo "The installation will be performed using yum package manager"
            elif [ -x "$(command -v pacman)" ]; then
                PACKAGE_MANAGER="pacman"
                echo "The installation will be performed using pacman package manager"
            fi
		;;
		Darwin)
            OS_NAME="macos"
			OS_TYPE="darwin"
            INSTALL_DIR="/usr/local/bin"
            
            # Check the availability of a compatible package manager
            if [ -x "$(command -v brew)" ]; then 
                PACKAGE_MANAGER="brew"
                echo "The installation will be performed using brew package manager"
            fi
		;;
	esac
    fi

    # Run the installation, if a desktop environment is not detected
    # only the CLI will be installed
    case "$PACKAGE_MANAGER" in
    apt)
        add_apt_repo
        sudo apt-get install netbird -y
        
        if ! $SKIP_UI_APP; then 
            sudo apt-get install netbird-ui -y
        fi
    ;;
    yum)
        add_rpm_repo
        sudo yum -y install netbird
        if ! $SKIP_UI_APP; then 
            sudo yum -y install netbird-ui
        fi
    ;;
    dnf)
        add_rpm_repo
        sudo dnf -y install dnf-plugin-config-manager
        sudo dnf config-manager --add-repo /etc/yum.repos.d/netbird.repo
        sudo dnf -y install netbird

        if ! $SKIP_UI_APP; then 
            sudo dnf -y install netbird-ui
        fi
    ;;
    pacman)
        sudo pacman -Syy
        add_aur_repo
    ;;
    brew)
        # Remove Wiretrustee if it had been installed using Homebrew before
        if brew ls --versions wiretrustee >/dev/null 2>&1; then
            echo "Removing existing wiretrustee client"
            
            # Stop and uninstall daemon service:
            wiretrustee service stop
            wiretrustee service uninstall 

            # Unlik the app
            brew unlink wiretrustee
        fi

        brew install netbirdio/tap/netbird
        if ! $SKIP_UI_APP; then 
            brew install --cask netbirdio/tap/netbird-ui
        fi
    ;;
    *)
        if [ "$OS_NAME" = "nixos" ];then
            echo "Please add Netbird to your NixOS configuration.nix directly:"
			echo
			echo "services.netbird.enable = true;"

            if ! $SKIP_UI_APP; then 
                 echo "environment.systemPackages = [ pkgs.netbird-ui ];"
            fi

            echo "Build and apply new configuration:"
            echo
            echo "sudo nixos-rebuild switch"
			exit 0
        fi   

        install_native_binaries
    ;;
    esac

    # Load and start netbird service
    if  ! sudo netbird service install 2>&1; then 
        echo "Netbird service has already been loaded"
    fi
    if  ! sudo netbird service start 2>&1; then 
        echo "Netbird service has already been started"
    fi


    echo "Installation has been finished. To connect, you need to run NetBird by executing the following command:"
    echo ""
    echo "sudo netbird up"
}

install_netbird