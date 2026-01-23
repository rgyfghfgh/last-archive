# The Last Archive - Professional CLI Tool for Windows
# Built for privacy-first web archival

# UI Components
function Show-Logo {
    Write-Host "`n    ___                __      _             " -ForegroundColor Cyan
    Write-Host "   /   |  _____________/ /_  __(_)_   _____  " -ForegroundColor Cyan
    Write-Host "  / /| | / ___/ ___/ __  / |/ / / | / / _ \ " -ForegroundColor Cyan
    Write-Host " / ___ |/ /  / /__/ /_/ /|  __/ /| |/ /  __/ " -ForegroundColor Cyan
    Write-Host "/_/  |_/_/   \___/\__,_/ |_/ /_/ |___/\___/  " -ForegroundColor Cyan
    Write-Host "         THE LAST ARCHIVE ENGINE" -ForegroundColor White
    Write-Host ""
}

function Log-Info ($msg) { Write-Host "[INFO] $msg" -ForegroundColor Cyan }
function Log-Success ($msg) { Write-Host "[SUCCESS] $msg" -ForegroundColor Green }
function Log-Warn ($msg) { Write-Host "[WARN] $msg" -ForegroundColor Yellow }
function Log-Error ($msg) { Write-Host "[ERROR] $msg" -ForegroundColor Red }

$PROJECT_ROOT = Get-Location
$NETWORK_NAME = "archive-network"
$SERVICES = @("server", "embedding_service", "llama-go", "frontend")

function Ensure-Network {
    $networks = docker network ls --filter name=$NETWORK_NAME -q
    if (-not $networks) {
        Log-Info "Creating internal network: $NETWORK_NAME..."
        docker network create $NETWORK_NAME
    }
}

function Check-SpiderEnv {
    $spiderEnvPath = "$PROJECT_ROOT\spider\.env"
    
    if (-not (Test-Path $spiderEnvPath)) {
        Log-Warn "Spider .env file not found. Creating with default values..."
        
        $envContent = @"
QDRANT_HOST=localhost
QDRANT_API_KEY=VERY_STRONG_KEY
"@
        
        Set-Content -Path $spiderEnvPath -Value $envContent
        Log-Success "Created spider/.env with default configuration"
    } else {
        Log-Info "Spider .env file already exists"
    }
}

function Wait-ForService {
    param(
        [string]$ServiceName,
        [int]$Timeout = 60
    )
    
    Log-Info "Waiting for $ServiceName to be healthy..."
    $elapsed = 0
    $interval = 2
    
    while ($elapsed -lt $Timeout) {
        $container = docker ps --filter "name=$ServiceName" --filter "status=running" --format "{{.Names}}"
        if ($container) {
            # Additional health check based on service type
            $healthy = $true
            
            switch -Wildcard ($ServiceName) {
                "*qdrant*" {
                    try {
                        $response = Invoke-WebRequest -Uri "http://localhost:6333/healthz" -TimeoutSec 2 -UseBasicParsing -ErrorAction SilentlyContinue
                        $healthy = $response.StatusCode -eq 200
                    } catch {
                        $healthy = $false
                    }
                }
                "*embedding*" {
                    try {
                        $response = Invoke-WebRequest -Uri "http://localhost:5050/health" -TimeoutSec 2 -UseBasicParsing -ErrorAction SilentlyContinue
                        $healthy = $response.StatusCode -eq 200
                    } catch {
                        $healthy = $false
                    }
                }
                "*llama*" {
                    try {
                        $response = Invoke-WebRequest -Uri "http://localhost:1410/health" -TimeoutSec 2 -UseBasicParsing -ErrorAction SilentlyContinue
                        $healthy = $response.StatusCode -eq 200
                    } catch {
                        $healthy = $false
                    }
                }
                "*server*" {
                    try {
                        $response = Invoke-WebRequest -Uri "http://localhost:1213/api/health" -TimeoutSec 2 -UseBasicParsing -ErrorAction SilentlyContinue
                        $healthy = $response.StatusCode -eq 200
                    } catch {
                        $healthy = $false
                    }
                }
                "*frontend*" {
                    try {
                        $response = Invoke-WebRequest -Uri "http://localhost:3000" -TimeoutSec 2 -UseBasicParsing -ErrorAction SilentlyContinue
                        $healthy = $true
                    } catch {
                        $healthy = $false
                    }
                }
            }
            
            if ($healthy) {
                Log-Success "$ServiceName is healthy and ready"
                return $true
            }
        }
        
        Start-Sleep -Seconds $interval
        $elapsed += $interval
        Write-Host "." -NoNewline -ForegroundColor Yellow
    }
    
    Write-Host ""
    Log-Warn "$ServiceName did not become healthy within $Timeout seconds"
    return $false
}

function Start-Services {
    Ensure-Network
    Check-SpiderEnv
    
    # Ensure Qdrant data directory exists
    if (-not (Test-Path "$PROJECT_ROOT\qdrant")) {
        New-Item -ItemType Directory -Force -Path "$PROJECT_ROOT\qdrant" | Out-Null
        Log-Info "Created qdrant data directory"
    }

    Log-Info "Starting services sequentially..."
    
    # Start server (which includes Qdrant via docker-compose)
    if (Test-Path "$PROJECT_ROOT\server") {
        Log-Info "Starting server with Qdrant..."
        Set-Location "$PROJECT_ROOT\server"
        docker-compose up -d
        Set-Location $PROJECT_ROOT
        
        # Wait for Qdrant first
        Wait-ForService -ServiceName "qdrant" -Timeout 120
        
        # Then wait for server
        Wait-ForService -ServiceName "server" -Timeout 120
    }
    
    # Start remaining services
    $remainingServices = @("embedding_service", "llama-go", "frontend")
    foreach ($service in $remainingServices) {
        if (Test-Path "$PROJECT_ROOT\$service") {
            Log-Info "Starting $service..."
            Set-Location "$PROJECT_ROOT\$service"
            docker-compose up -d
            Set-Location $PROJECT_ROOT
            
            # Wait for service to be healthy before continuing
            Wait-ForService -ServiceName $service -Timeout 120
        }
    }
    
    Log-Success "All systems are operational."
    Write-Host "`n  Frontend: http://localhost:3000"
    Write-Host "  API:      http://localhost:1213`n"
}

function Stop-Services {
    Log-Info "Gracefully terminating services..."
    $REVERSE_SERVICES = @("frontend", "llama-go", "embedding_service", "server")
    
    foreach ($service in $REVERSE_SERVICES) {
        if (Test-Path "$PROJECT_ROOT\$service") {
            Log-Info "Stopping $service..."
            Set-Location "$PROJECT_ROOT\$service"
            docker-compose down
            Set-Location $PROJECT_ROOT
        }
    }
    Log-Success "All services stopped."
}

function Install-GCC {
    Log-Info "GCC not found. Checking for installation options..."
    
    # Check if Chocolatey is available
    if (Get-Command choco -ErrorAction SilentlyContinue) {
        Log-Info "Installing MinGW via Chocolatey..."
        choco install mingw -y
        
        # Refresh environment variables
        $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
        
        Log-Success "GCC installed successfully via Chocolatey"
        return $true
    }
    
    # Check if winget is available (Windows 10+)
    if (Get-Command winget -ErrorAction SilentlyContinue) {
        Log-Info "Installing MinGW via winget..."
        winget install -e --id=MSYS2.MSYS2 --silent --accept-package-agreements --accept-source-agreements
        
        # Wait for installation to complete
        Start-Sleep -Seconds 3
        
        # Add MSYS2 MinGW to PATH temporarily
        $msys2Path = "C:\msys64\mingw64\bin"
        $msys2Usr = "C:\msys64\usr\bin"
        
        if (Test-Path $msys2Usr) {
            $env:Path += ";$msys2Usr;$msys2Path"
            
            Log-Info "Installing GCC via MSYS2..."
            # Use proper bash command syntax
            & "$msys2Usr\bash.exe" -lc "pacman -Sy --noconfirm mingw-w64-x86_64-gcc"
            
            # Verify GCC was installed
            if (Test-Path "$msys2Path\gcc.exe") {
                Log-Success "GCC installed successfully via winget/MSYS2"
                Log-Info "Adding to system PATH permanently..."
                
                # Add to user PATH permanently
                $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
                if ($userPath -notlike "*$msys2Path*") {
                    [Environment]::SetEnvironmentVariable("Path", "$userPath;$msys2Path", "User")
                }
                
                return $true
            } else {
                Log-Error "GCC installation failed"
                return $false
            }
        } else {
            Log-Error "MSYS2 installation failed or not found"
            return $false
        }
    }
    
    # Manual download as last resort
    Log-Warn "Automated package managers not found."
    Log-Warn "Please install GCC manually using one of these options:"
    Write-Host "  1. TDM-GCC: https://jmeubank.github.io/tdm-gcc/" -ForegroundColor Yellow
    Write-Host "  2. MSYS2: https://www.msys2.org/" -ForegroundColor Yellow
    Write-Host "  3. Or install Chocolatey first: https://chocolatey.org/install" -ForegroundColor Yellow
    
    return $false
}

function Run-Crawler {
    Show-Logo
    $urls = Read-Host "Enter seed URLs (separated by space)"
    
    if (-not $urls) {
        Log-Error "No URLs provided. Aborting."
        return
    }

    $urlArray = $urls.Split(" ", [System.StringSplitOptions]::RemoveEmptyEntries)
    
    Log-Info "Initializing spider bot..."
    if (Test-Path "$PROJECT_ROOT\spider") {
        Set-Location "$PROJECT_ROOT\spider"
        
        # Check if go is available
        if (Get-Command go -ErrorAction SilentlyContinue) {
            # Check if GCC is available (required for CGO)
            if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) {
                Log-Warn "GCC compiler not found (required for SQLite support)"
                $install = Read-Host "Would you like to install GCC automatically? (y/n)"
                
                if ($install -eq 'y' -or $install -eq 'Y') {
                    if (Install-GCC) {
                        Log-Info "Please restart PowerShell and run the crawler again."
                        Set-Location $PROJECT_ROOT
                        return
                    } else {
                        Log-Warn "Falling back to Docker..."
                        docker-compose run spider $urlArray
                        Set-Location $PROJECT_ROOT
                        return
                    }
                } else {
                    Log-Warn "GCC not installed. Falling back to Docker..."
                    docker-compose run spider $urlArray
                    Set-Location $PROJECT_ROOT
                    return
                }
            }
            
            # Enable CGO for SQLite support
            $env:CGO_ENABLED = "1"
            go run main.go $urlArray
        } else {
            Log-Warn "Go not found, attempting to run via Docker..."
            docker-compose run spider $urlArray
        }
        Set-Location $PROJECT_ROOT
    }
}

function Show-Status {
    Log-Info "Current Service Status:"
    docker ps --filter "network=$NETWORK_NAME" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
}

# Command Dispatcher
$command = $args[0]

switch ($command) {
    "up" {
        Show-Logo
        Start-Services
    }
    "down" {
        Stop-Services
    }
    "crawl" {
        Run-Crawler
    }
    "status" {
        Show-Logo
        Show-Status
    }
    "logs" {
        $containers = docker ps --filter "network=$NETWORK_NAME" --format "{{.Names}}"
        foreach ($container in $containers) {
            Write-Host "--- Logs: $container ---" -ForegroundColor Cyan
            docker logs --tail 50 $container
        }
    }
    "prune" {
        Show-Logo
        Log-Warn "This will remove stopped containers and unused networks."
        $confirm = Read-Host "Are you sure? (y/n)"
        if ($confirm -eq 'y') {
            docker system prune -f
            Log-Success "System pruned."
        }
    }
    Default {
        Show-Logo
        Write-Host "Usage: .\archive.ps1 [COMMAND]`n"
        Write-Host "Commands:"
        Write-Host "  up      Start all microservices"
        Write-Host "  down    Stop all microservices"
        Write-Host "  crawl   Run the web crawler with custom seeds"
        Write-Host "  status  Check system health"
        Write-Host "  logs    Show logs from all services"
        Write-Host "  prune   Remove unused docker data"
    }
}