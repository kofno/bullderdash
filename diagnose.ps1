# Debugging Bull-der-dash Issues

Write-Host "🔍 Bull-der-dash Diagnostic Report" -ForegroundColor Cyan
Write-Host "=================================" -ForegroundColor Cyan
Write-Host ""

# Check Valkey
Write-Host "1. Checking Valkey/Redis..." -ForegroundColor Yellow
$redisRunning = Get-Process redis-server -ErrorAction SilentlyContinue
if ($redisRunning) {
    Write-Host "✅ Redis process running" -ForegroundColor Green
    try {
        $ping = & redis-cli ping 2>&1
        Write-Host "✅ redis-cli ping: $ping" -ForegroundColor Green
    } catch {
        Write-Host "❌ redis-cli error: $_" -ForegroundColor Red
    }
} else {
    Write-Host "❌ Redis/Valkey not found" -ForegroundColor Red
    Write-Host "Available Docker containers:" -ForegroundColor Gray
    docker ps | grep -i valkey
}

Write-Host ""
Write-Host "2. Checking Bull-der-dash..." -ForegroundColor Yellow

# Try to connect to port 8080
try {
    $socket = New-Object System.Net.Sockets.TcpClient
    $socket.Connect("127.0.0.1", 8080)
    $socket.Close()
    Write-Host "✅ Port 8080 is open" -ForegroundColor Green
} catch {
    Write-Host "❌ Port 8080 is closed - app may not be running" -ForegroundColor Red
}

Write-Host ""
Write-Host "3. Testing endpoints..." -ForegroundColor Yellow

# Health check
try {
    $health = Invoke-WebRequest -Uri "http://localhost:8080/health" -ErrorAction Stop
    Write-Host "✅ /health: $($health.StatusCode) - $($health.Content)" -ForegroundColor Green
} catch {
    Write-Host "❌ /health: $($_)" -ForegroundColor Red
}

# Ready check
try {
    $ready = Invoke-WebRequest -Uri "http://localhost:8080/ready" -ErrorAction Stop
    Write-Host "✅ /ready: $($ready.StatusCode) - $($ready.Content)" -ForegroundColor Green
} catch {
    Write-Host "❌ /ready: $($_)" -ForegroundColor Red
}

# Queues endpoint
try {
    $queues = Invoke-WebRequest -Uri "http://localhost:8080/queues" -ErrorAction Stop
    Write-Host "✅ /queues: $($queues.StatusCode)" -ForegroundColor Green
    Write-Host "   Response size: $($queues.Content.Length) bytes" -ForegroundColor Gray
    Write-Host "   First 500 chars:" -ForegroundColor Gray
    Write-Host ($queues.Content -replace '<[^>]+>', '' -replace '\s+', ' ' -replace '^(.{500}).*', '$1...') -ForegroundColor Gray
} catch {
    Write-Host "❌ /queues: $($_)" -ForegroundColor Red
}

# Metrics
try {
    $metrics = Invoke-WebRequest -Uri "http://localhost:8080/metrics" -ErrorAction Stop
    $bullmq = $metrics.Content | Select-String "bullmq" | Measure-Object | Select-Object -ExpandProperty Count
    Write-Host "✅ /metrics: $($metrics.StatusCode)" -ForegroundColor Green
    Write-Host "   Found $bullmq BullMQ metrics" -ForegroundColor Gray
} catch {
    Write-Host "❌ /metrics: $($_)" -ForegroundColor Red
}

Write-Host ""
Write-Host "4. Checking Redis data..." -ForegroundColor Yellow
try {
    $keys = & redis-cli KEYS "bull:*:id" | Measure-Object | Select-Object -ExpandProperty Count
    Write-Host "✅ Found $keys queue IDs in Redis" -ForegroundColor Green

    $allBullKeys = & redis-cli KEYS "bull:*" | Measure-Object | Select-Object -ExpandProperty Count
    Write-Host "✅ Found $allBullKeys total BullMQ keys" -ForegroundColor Green

    & redis-cli KEYS "bull:*:id" | ForEach-Object { Write-Host "   - $_" -ForegroundColor Gray }
} catch {
    Write-Host "❌ Redis keys error: $_" -ForegroundColor Red
}

Write-Host ""
Write-Host "=================================" -ForegroundColor Cyan
Write-Host "Diagnostic report complete" -ForegroundColor Cyan

