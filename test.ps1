# Test script for Bull-der-dash

Write-Host "🧪 Testing Bull-der-dash" -ForegroundColor Cyan
Write-Host ""

Write-Host "1️⃣ Testing health endpoint..." -ForegroundColor Yellow
try {
    $response = curl -s http://localhost:8080/health
    Write-Host "Response: $response" -ForegroundColor Green
} catch {
    Write-Host "Error: $_" -ForegroundColor Red
}
Write-Host ""

Write-Host "2️⃣ Testing ready endpoint..." -ForegroundColor Yellow
try {
    $response = curl -s http://localhost:8080/ready
    Write-Host "Response: $response" -ForegroundColor Green
} catch {
    Write-Host "Error: $_" -ForegroundColor Red
}
Write-Host ""

Write-Host "3️⃣ Testing queues endpoint..." -ForegroundColor Yellow
try {
    $response = curl -s http://localhost:8080/queues
    if ($response.Length -gt 0) {
        Write-Host "✅ Got response ($(($response | Measure-Object -Character).Characters) chars)" -ForegroundColor Green
        Write-Host "First 200 chars:" -ForegroundColor Gray
        Write-Host ($response | Select-Object -First 200) -ForegroundColor Gray
    } else {
        Write-Host "❌ Empty response" -ForegroundColor Red
    }
} catch {
    Write-Host "Error: $_" -ForegroundColor Red
}
Write-Host ""

Write-Host "4️⃣ Testing metrics endpoint..." -ForegroundColor Yellow
try {
    $response = curl -s http://localhost:8080/metrics
    $bullmqLines = $response | Select-String "bullmq" -AllMatches | Select-Object -First 5
    if ($bullmqLines) {
        Write-Host "✅ Found Prometheus metrics:" -ForegroundColor Green
        $bullmqLines | ForEach-Object { Write-Host $_ -ForegroundColor Gray }
    } else {
        Write-Host "⚠️ No bullmq metrics found" -ForegroundColor Yellow
    }
} catch {
    Write-Host "Error: $_" -ForegroundColor Red
}
Write-Host ""

Write-Host "5️⃣ Checking Valkey connection..." -ForegroundColor Yellow
try {
    $pong = & redis-cli ping
    Write-Host "✅ Valkey is running: $pong" -ForegroundColor Green

    $queueKeys = & redis-cli KEYS "bull:*:id" | Measure-Object -Line
    Write-Host "✅ Found $($queueKeys.Lines) queues in Valkey" -ForegroundColor Green
} catch {
    Write-Host "Error: $_" -ForegroundColor Red
}

