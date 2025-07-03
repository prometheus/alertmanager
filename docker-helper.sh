#!/usr/bin/env bash

# Helper script to manage Alertmanager Docker Compose setup

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to start services
start_services() {
    log_info "Starting Alertmanager cluster and webhook server..."
    docker compose up -d
    
    log_info "Waiting for services to be ready..."
    sleep 10
    
    # Check if services are healthy
    log_info "Checking service health..."
    docker compose ps
    
    echo
    log_info "Services are running!"
    echo "Alertmanager instances:"
    echo "  - Instance 1: http://localhost:9093"
    echo "  - Instance 2: http://localhost:9094"
    echo "  - Instance 3: http://localhost:9095"
    echo "Webhook server: http://localhost:5001"
}

# Function to stop services
stop_services() {
    log_info "Stopping Alertmanager cluster..."
    docker compose down
    log_info "Services stopped."
}

# Function to restart services
restart_services() {
    log_info "Restarting Alertmanager cluster..."
    docker compose restart
    log_info "Services restarted."
}

# Function to show logs
show_logs() {
    if [ -n "$1" ]; then
        docker compose logs -f "$1"
    else
        docker compose logs -f
    fi
}

# Function to send test alerts
send_alerts() {
    log_info "Sending test alerts..."
    
    # Check if send_alerts.sh exists
    if [ ! -f "examples/ha/send_alerts.sh" ]; then
        log_error "send_alerts.sh not found at examples/ha/send_alerts.sh"
        exit 1
    fi
    
    # Make it executable
    chmod +x examples/ha/send_alerts.sh
    
    # Run the script
    ./examples/ha/send_alerts.sh
    
    log_info "Test alerts sent!"
    log_info "Check webhook server logs: docker compose logs webhook-server"
}

# Function to check service status
check_status() {
    log_info "Checking service status..."
    docker compose ps
    
    echo
    log_info "Service endpoints:"
    echo "  - Alertmanager 1: http://localhost:9093"
    echo "  - Alertmanager 2: http://localhost:9094"
    echo "  - Alertmanager 3: http://localhost:9095"
    echo "  - Webhook server: http://localhost:5001"
    
    echo
    log_info "Health checks:"
    for port in 9093 9094 9095; do
        if curl -s -o /dev/null -w "%{http_code}" "http://localhost:$port/-/healthy" | grep -q "200"; then
            echo -e "  - Alertmanager ($port): ${GREEN}✓ Healthy${NC}"
        else
            echo -e "  - Alertmanager ($port): ${RED}✗ Unhealthy${NC}"
        fi
    done
    
    if curl -s -o /dev/null -w "%{http_code}" "http://localhost:5001/" | grep -q "200"; then
        echo -e "  - Webhook server: ${GREEN}✓ Healthy${NC}"
    else
        echo -e "  - Webhook server: ${RED}✗ Unhealthy${NC}"
    fi
}

# Function to clean up everything
cleanup() {
    log_info "Cleaning up containers, images, and volumes..."
    docker compose down -v --rmi all
    log_info "Cleanup complete."
}

# Main function
main() {
    case "${1:-}" in
        start)
            start_services
            ;;
        stop)
            stop_services
            ;;
        restart)
            restart_services
            ;;
        logs)
            show_logs "$2"
            ;;
        send-alerts)
            send_alerts
            ;;
        status)
            check_status
            ;;
        cleanup)
            cleanup
            ;;
        *)
            echo "Usage: $0 {start|stop|restart|logs [service]|send-alerts|status|cleanup}"
            echo
            echo "Commands:"
            echo "  start       - Start all services"
            echo "  stop        - Stop all services"
            echo "  restart     - Restart all services"
            echo "  logs        - Show logs for all services"
            echo "  logs <svc>  - Show logs for specific service"
            echo "  send-alerts - Send test alerts"
            echo "  status      - Check service status"
            echo "  cleanup     - Remove containers, images, and volumes"
            echo
            echo "Examples:"
            echo "  $0 start"
            echo "  $0 logs webhook-server"
            echo "  $0 send-alerts"
            exit 1
            ;;
    esac
}

# Run main function
main "$@"
