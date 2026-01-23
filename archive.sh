#!/bin/bash
# The Last Archive - Professional CLI Tool
# Built for privacy-first web archival
set -e

# Configuration
PROJECT_ROOT=$(pwd)
NETWORK_NAME="archive-network"
SPIDER_DIR="$PROJECT_ROOT/spider"

COLOR_CYAN='\033[0;36m'
COLOR_GREEN='\033[0;32m'
COLOR_RED='\033[0;31m'
COLOR_YELLOW='\033[1;33m'
COLOR_WHITE='\033[1;37m'
COLOR_NC='\033[0m'

# UI Components
show_logo() {
    echo -e "${COLOR_CYAN}"
    echo "    ___                __      _             "
    echo "   /   |  _____________/ /_  __(_)_   _____  "
    echo "  / /| | / ___/ ___/ __  / |/ / / | / / _ \ "
    echo " / ___ |/ /  / /__/ /_/ /|  __/ /| |/ /  __/ "
    echo "/_/  |_/_/   \___/\__,_/ |_/ /_/ |___/\___/  "
    echo -e "         ${COLOR_WHITE}THE LAST ARCHIVE ENGINE${COLOR_NC}"
    echo ""
}

log_info() { echo -e "${COLOR_CYAN}[INFO]${COLOR_NC} $1"; }
log_success() { echo -e "${COLOR_GREEN}[SUCCESS]${COLOR_NC} $1"; }
log_warn() { echo -e "${COLOR_YELLOW}[WARN]${COLOR_NC} $1"; }
log_error() { echo -e "${COLOR_RED}[ERROR]${COLOR_NC} $1"; }

# Core Functions
ensure_network() {
    if ! docker network ls | grep -q "$NETWORK_NAME"; then
        log_info "Creating internal network: $NETWORK_NAME..."
        docker network create "$NETWORK_NAME"
    fi
}

check_spider_env() {
    local spider_env_path="$SPIDER_DIR/.env"
    
    if [ ! -f "$spider_env_path" ]; then
        log_warn "Spider .env file not found. Creating with default values..."
        
        cat > "$spider_env_path" << EOF
QDRANT_HOST=localhost
QDRANT_API_KEY=VERY_STRONG_KEY
EOF
        
        log_success "Created spider/.env with default configuration"
    else
        log_info "Spider .env file already exists"
    fi
}

wait_for_service() {
    local service_name=$1
    local timeout=${2:-60}
    local elapsed=0
    local interval=2
    
    log_info "Waiting for $service_name to be healthy..."
    
    while [ $elapsed -lt $timeout ]; do
        if docker ps --filter "name=$service_name" --filter "status=running" --format "{{.Names}}" | grep -q "$service_name"; then
            local healthy=true
            
            case $service_name in
                *qdrant*)
                    if ! curl -s -f http://localhost:6333/healthz > /dev/null 2>&1; then
                        healthy=false
                    fi
                    ;;
                *embedding*)
                    if ! curl -s -f http://localhost:5050/health > /dev/null 2>&1; then
                        healthy=false
                    fi
                    ;;
                *llama*)
                    if ! curl -s -f http://localhost:1410/health > /dev/null 2>&1; then
                        healthy=false
                    fi
                    ;;
                *server*)
                    if ! curl -s -f http://localhost:1213/api/health > /dev/null 2>&1; then
                        healthy=false
                    fi
                    ;;
                *frontend*)
                    if ! curl -s -f http://localhost:3000 > /dev/null 2>&1; then
                        healthy=false
                    fi
                    ;;
            esac
            
            if [ "$healthy" = true ]; then
                log_success "$service_name is healthy and ready"
                return 0
            fi
        fi
        
        sleep $interval
        elapsed=$((elapsed + interval))
        echo -n "." >&2
    done
    
    echo ""
    log_warn "$service_name did not become healthy within $timeout seconds"
    return 1
}

start_services() {
    ensure_network
    check_spider_env
    
    if [ ! -d "$PROJECT_ROOT/qdrant" ]; then
        mkdir -p "$PROJECT_ROOT/qdrant"
        log_info "Created qdrant data directory"
    fi

    log_info "Starting services sequentially..."
    
    # Start server (which includes Qdrant via docker-compose)
    if [ -d "$PROJECT_ROOT/server" ]; then
        log_info "Starting server with Qdrant..."
        cd "$PROJECT_ROOT/server" && docker-compose up -d
        cd "$PROJECT_ROOT"
        
        # Wait for Qdrant first
        wait_for_service "qdrant" 120
        
        # Then wait for server
        wait_for_service "server" 120
    fi
    
    # Start remaining services
    REMAINING_SERVICES=("embedding_service" "llama-go" "frontend")
    
    for service in "${REMAINING_SERVICES[@]}"; do
        if [ -d "$PROJECT_ROOT/$service" ]; then
            log_info "Starting $service..."
            cd "$PROJECT_ROOT/$service" && docker-compose up -d
            cd "$PROJECT_ROOT"
            
            # Wait for service to be healthy before continuing
            wait_for_service "$service" 120
        fi
    done
    
    log_success "All systems are operational."
    echo -e "\n  Frontend: http://localhost:3000"
    echo -e "  API:      http://localhost:1213\n"
}

stop_services() {
    log_info "Gracefully terminating services..."
    SERVICES=("frontend" "llama-go" "embedding_service" "server")
    
    for service in "${SERVICES[@]}"; do
        if [ -d "$PROJECT_ROOT/$service" ]; then
            cd "$PROJECT_ROOT/$service" && docker-compose down
        fi
    done
    log_success "All services stopped."
}

run_crawler() {
    show_logo
    echo -e "${COLOR_YELLOW}Enter seed URLs (separated by space):${COLOR_NC}"
    read -p "> " -a URLS
    
    if [ ${#URLS[@]} -eq 0 ]; then
        log_error "No URLs provided. Aborting."
        exit 1
    fi

    log_info "Initializing spider bot..."
    cd "$SPIDER_DIR"
    
    # Check if we should run via Go or Docker
    if command -v go &> /dev/null; then
        # Enable CGO for SQLite support
        CGO_ENABLED=1 go run main.go "${URLS[@]}"
    else
        log_warn "Go not found, attempting to run via Docker..."
        docker-compose run spider "${URLS[@]}"
    fi
}

show_status() {
    log_info "Current Service Status:"
    docker ps --filter "network=$NETWORK_NAME" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
}

# Command Dispatcher
case "$1" in
    up)
        show_logo
        start_services
        ;;
    down)
        stop_services
        ;;
    crawl)
        run_crawler
        ;;
    status)
        show_logo
        show_status
        ;;
    logs)
        docker ps --filter "network=$NETWORK_NAME" --format "{{.Names}}" | xargs -I {} docker logs --tail 50 -f {}
        ;;
    prune)
        show_logo
        log_warn "This will remove stopped containers and unused networks."
        read -p "Are you sure? (y/n) " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            docker system prune -f
            log_success "System pruned."
        fi
        ;;
    *)
        show_logo
        echo "Usage: ./archive.sh [COMMAND]"
        echo ""
        echo "Commands:"
        echo "  up      Start all microservices"
        echo "  down    Stop all microservices"
        echo "  crawl   Run the web crawler with custom seeds"
        echo "  status  Check system health"
        echo "  logs    Stream logs from all services"
        echo "  prune   Remove unused docker data"
        echo ""
        ;;
esac