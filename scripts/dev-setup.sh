#!/usr/bin/env bash
#
# Unified Development Environment Setup Script
# Sets up Go, Python, and Web development environments
#
# Usage:
#   ./scripts/dev-setup.sh [--skip-go] [--skip-python] [--skip-web] [--skip-deps]
#

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default options
SKIP_GO=false
SKIP_PYTHON=false
SKIP_WEB=false
SKIP_DEPS=false

LOG_ROOT="${TMPDIR:-/tmp}/velocity.dev-setup"
LOG_DIR="$LOG_ROOT/$(date +%Y%m%d-%H%M%S)"
mkdir -p "$LOG_DIR"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --skip-go)
            SKIP_GO=true
            shift
            ;;
        --skip-python)
            SKIP_PYTHON=true
            shift
            ;;
        --skip-web)
            SKIP_WEB=true
            shift
            ;;
        --skip-deps)
            SKIP_DEPS=true
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --skip-go       Skip Go environment setup"
            echo "  --skip-python   Skip Python environment setup"
            echo "  --skip-web      Skip Web environment setup"
            echo "  --skip-deps     Skip system dependency checks"
            echo "  -h, --help      Show this help message"
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Helper functions
print_step() {
    echo -e "\n${BLUE}==>${NC} ${GREEN}$1${NC}"
}

print_info() {
    echo -e "${BLUE}info:${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}Warning:${NC} $1"
}

print_error() {
    echo -e "${RED}Error:${NC} $1"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

slugify() {
    echo "$1" | tr '[:upper:]' '[:lower:]' | tr -cs '[:alnum:]' '-'
}

run_with_log() {
    local description="$1"
    shift
    local slug
    slug=$(slugify "$description")
    local log_file="$LOG_DIR/${slug}.log"

    print_step "$description"

    set +e
    "$@" >"$log_file" 2>&1
    local status=$?
    set -e

    if [ $status -eq 0 ]; then
        print_success "$description"
    else
        print_error "$description failed (see $log_file)"
        if [ -s "$log_file" ]; then
            echo "-- log tail --"
            tail -n 20 "$log_file" || true
            echo "-- end log tail --"
        fi
        exit $status
    fi
}

# Detect OS
detect_os() {
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        if [ -f /etc/os-release ]; then
            . /etc/os-release
            OS=$ID
            OS_VERSION=$VERSION_ID
        else
            OS="unknown"
        fi
    elif [[ "$OSTYPE" == "darwin"* ]]; then
        OS="macos"
        OS_VERSION=$(sw_vers -productVersion 2>/dev/null || echo "unknown")
    else
        OS="unknown"
    fi

    if [ -n "$OS_VERSION" ]; then
        print_step "Detected OS: $OS $OS_VERSION"
    else
        print_step "Detected OS: $OS"
    fi
}

# Check system dependencies
check_system_deps() {
    if [ "$SKIP_DEPS" = true ]; then
        print_warning "Skipping system dependency checks"
        return
    fi

    print_step "Checking system dependencies"

    local missing_deps=()

    # Required tools
    if ! command_exists git; then
        missing_deps+=("git")
    else
        print_success "git: $(git --version)"
    fi

    if ! command_exists make; then
        missing_deps+=("make")
    else
        print_success "make: $(make --version | head -n1)"
    fi

    if ! command_exists curl; then
        missing_deps+=("curl")
    else
        print_success "curl: $(curl --version | head -n1)"
    fi

    if [ ${#missing_deps[@]} -gt 0 ]; then
        print_error "Missing required dependencies: ${missing_deps[*]}"
        echo ""
        echo "Please install them and run this script again:"
        if [[ "$OS" == "ubuntu" ]] || [[ "$OS" == "debian" ]]; then
            echo "  sudo apt-get update"
            echo "  sudo apt-get install ${missing_deps[*]}"
        elif [[ "$OS" == "macos" ]]; then
            echo "  brew install ${missing_deps[*]}"
        fi
        exit 1
    fi
}

# Setup Go environment
setup_go() {
    if [ "$SKIP_GO" = true ]; then
        print_warning "Skipping Go setup"
        return
    fi

    print_step "Setting up Go development environment"

    # Check Go installation
    if ! command_exists go; then
        print_error "Go is not installed"
        echo ""
        echo "Please install Go 1.21 or later:"
        if [[ "$OS" == "ubuntu" ]] || [[ "$OS" == "debian" ]]; then
            echo "  wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz"
            echo "  sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz"
            echo "  echo 'export PATH=\$PATH:/usr/local/go/bin' >> ~/.bashrc"
        elif [[ "$OS" == "macos" ]]; then
            echo "  brew install go"
        else
            echo "  Visit https://go.dev/dl/"
        fi
        exit 1
    fi

    local go_version=$(go version | awk '{print $3}')
    print_success "Go installed: $go_version"

    # Check Go version
    local required_version="go1.21"
    if [[ "$go_version" < "$required_version" ]]; then
        print_warning "Go version should be 1.21 or later (found $go_version)"
    fi

    # Download Go dependencies
    run_with_log "Downloading Go dependencies" go mod download

    # Verify build
    run_with_log "Running go build smoke test" bash -c "go build -o /tmp/test-build ./cmd/radar"
    rm -f /tmp/test-build

    # Install development tools
    print_step "Installing Go development tools"

    if ! command_exists golangci-lint; then
        run_with_log "Installing golangci-lint" bash -c "curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin"
    else
        print_success "golangci-lint already installed"
    fi
}

# Setup Python environment
setup_python() {
    if [ "$SKIP_PYTHON" = true ]; then
        print_warning "Skipping Python setup"
        return
    fi

    print_step "Setting up Python development environment"

    # Check Python installation
    if ! command_exists python3; then
        print_error "Python 3 is not installed"
        echo ""
        echo "Please install Python 3.9 or later:"
        if [[ "$OS" == "ubuntu" ]] || [[ "$OS" == "debian" ]]; then
            echo "  sudo apt-get update"
            echo "  sudo apt-get install python3 python3-venv python3-pip"
        elif [[ "$OS" == "macos" ]]; then
            echo "  brew install python@3.11"
        fi
        exit 1
    fi

    local python_version=$(python3 --version)
    print_success "Python installed: $python_version"

    # Create virtual environment at repository root
    if [ ! -d ".venv" ]; then
        run_with_log "Creating Python virtual environment" python3 -m venv .venv
    else
        print_success "Virtual environment already exists"
    fi

    # Activate virtual environment and install dependencies
    source .venv/bin/activate

    # Upgrade pip
    run_with_log "Upgrading pip" pip install --upgrade pip

    # Install dependencies
    if [ -f "requirements.txt" ]; then
        run_with_log "Installing python requirements" pip install -r requirements.txt
    fi

    # Check LaTeX installation
    print_step "Checking LaTeX installation"
    if command_exists xelatex; then
        print_success "XeLaTeX installed: $(xelatex --version | head -n1)"
    else
        print_warning "XeLaTeX not found - required for PDF generation"
        echo ""
        echo "Install with:"
        if [[ "$OS" == "ubuntu" ]] || [[ "$OS" == "debian" ]]; then
            echo "  sudo apt-get install texlive-xetex texlive-fonts-extra"
        elif [[ "$OS" == "macos" ]]; then
            echo "  brew install --cask mactex"
            echo "  # Or for minimal install:"
            echo "  brew install basictex"
        fi
    fi

    deactivate
}

# Setup Web environment
setup_web() {
    if [ "$SKIP_WEB" = true ]; then
        print_warning "Skipping Web setup"
        return
    fi

    print_step "Setting up Web development environment"

    cd web

    # Check Node.js installation
    if ! command_exists node; then
        print_error "Node.js is not installed"
        echo ""
        echo "Please install Node.js 18 or later:"
        if [[ "$OS" == "ubuntu" ]] || [[ "$OS" == "debian" ]]; then
            echo "  curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -"
            echo "  sudo apt-get install -y nodejs"
        elif [[ "$OS" == "macos" ]]; then
            echo "  brew install node"
        fi
        exit 1
    fi

    local node_version=$(node --version)
    print_success "Node.js installed: $node_version"

    # Check pnpm installation
    if ! command_exists pnpm; then
        run_with_log "Installing pnpm" npm install -g pnpm
    else
        local pnpm_version=$(pnpm --version)
        print_success "pnpm installed: v$pnpm_version"
    fi

    # Install dependencies
    run_with_log "Installing web dependencies" pnpm install

    # Verify build
    local web_build_log="$LOG_DIR/web-build.log"
    print_step "Verifying web build"
    if pnpm run build > "$web_build_log" 2>&1; then
        print_success "Web build successful"
        rm -f "$web_build_log"
    else
        print_warning "Web build failed (see $web_build_log)"
        if [ -s "$web_build_log" ]; then
            echo "-- log tail --"
            tail -n 20 "$web_build_log" || true
            echo "-- end log tail --"
        fi
    fi

    cd ..
}

# Create database if missing
setup_database() {
    print_step "Setting up database"

    if [ ! -f "sensor_data.db" ]; then
        print_step "Creating database"
        run_with_log "Applying database schema" bash -c "sqlite3 sensor_data.db < internal/db/schema.sql"
    else
        print_success "Database already exists"
    fi

    # Verify database integrity
    local db_integrity_log="$LOG_DIR/sqlite-integrity.log"
    if sqlite3 sensor_data.db "PRAGMA integrity_check;" > "$db_integrity_log" 2>&1; then
        print_success "Database integrity check passed"
        rm -f "$db_integrity_log"
    else
        print_warning "Database integrity check failed (see $db_integrity_log)"
        if [ -s "$db_integrity_log" ]; then
            echo "-- log tail --"
            tail -n 20 "$db_integrity_log" || true
            echo "-- end log tail --"
        fi
    fi
}

# Create example configs
create_example_configs() {
    print_step "Creating example configuration files"

    if [ ! -f "tools/pdf-generator/config.example.json" ]; then
        source .venv/bin/activate
        cd tools/pdf-generator
        run_with_log "Generating config.example.json" python -m pdf_generator.cli.create_config --output config.example.json
        cd ../..
        deactivate
    else
        print_success "config.example.json already exists"
    fi
}

# Print next steps
print_next_steps() {
    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}✓ Development environment setup complete!${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo "Next steps:"
    echo ""

    if [ "$SKIP_GO" = false ]; then
        echo -e "${BLUE}Go Server:${NC}"
        echo "  make build          # Build binaries"
        echo "  make run            # Run server (requires sensors)"
        echo "  make test           # Run tests"
        echo ""
    fi

    if [ "$SKIP_PYTHON" = false ]; then
        echo -e "${BLUE}Python PDF Generator:${NC}"
        echo "  source .venv/bin/activate"
        echo "  cd tools/pdf-generator"
        echo "  python -m pdf_generator.cli.create_config --output my-config.json"
        echo "  # Edit my-config.json with your values"
        echo "  python internal/report/query_data/get_stats.py my-config.json"
        echo ""
    fi

    if [ "$SKIP_WEB" = false ]; then
        echo -e "${BLUE}Web Frontend:${NC}"
        echo "  cd web"
        echo "  pnpm run dev        # Start development server"
        echo "  pnpm run build      # Build for production"
        echo "  pnpm run test       # Run tests"
        echo ""
    fi

    echo -e "${BLUE}Documentation:${NC}"
    echo "  README.md           # Project overview"
    echo "  ARCHITECTURE.md     # System architecture"
    echo "  CONTRIBUTING.md     # Development guide"
    echo "  TROUBLESHOOTING.md  # Common issues"
    echo "  PERFORMANCE.md      # Performance tuning"
    echo ""

    echo -e "${BLUE}Code Formatting:${NC}"
    echo "  make format                # Format code before committing"
    echo "  make lint                  # Check formatting (what CI checks)"
    echo ""

    echo -e "${BLUE}Optional: Enable pre-commit hooks${NC}"
    echo "  pip install pre-commit && pre-commit install"
    echo "  # Auto-formats code on every commit (recommended for regular contributors)"
    echo ""

    if ! command_exists xelatex; then
        echo -e "${YELLOW}Note:${NC} XeLaTeX is not installed. PDF generation will not work."
        echo "See setup output above for installation instructions."
        echo ""
    fi

    echo -e "Logs saved under: $LOG_DIR"
}

# Main execution
main() {
    echo -e "${BLUE}=======================================${NC}"
    echo -e "${BLUE}Velocity Report - Development Setup${NC}"
    echo -e "${BLUE}=======================================${NC}"
    print_info "Log directory: $LOG_DIR"
    print_info "Options: skip_go=$SKIP_GO skip_python=$SKIP_PYTHON skip_web=$SKIP_WEB skip_deps=$SKIP_DEPS"

    # Detect OS
    detect_os

    # Check system dependencies
    check_system_deps

    # Setup each component
    setup_go
    setup_python
    setup_web

    # Setup database
    setup_database

    # Create example configs
    create_example_configs

    # Print next steps
    print_next_steps
}

# Run main function
main
