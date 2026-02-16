# Quick Test Script

Write-Host "Starting Bull-der-dash Dashboard..." -ForegroundColor Cyan

# Start the dashboard
$dashboard = Start-Process -FilePath ".\bullderdash.exe" -NoNewWindow -PassThru

# Wait for it to start
Start-Sleep -Seconds 2

# Test the endpoints
Write-Host "Testing /queues endpoint..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "http://localhost:8080/queues" -ErrorAction Stop
    Write-Host "✅ Success! Status: $($response.StatusCode)" -ForegroundColor Green
    Write-Host "Response contains HTML table: $(($response.Content -like '*<table*').ToString())" -ForegroundColor Green

    # Show first few lines
    $lines = $response.Content -split '\n' | Select-Object -First 5
    Write-Host "First few lines:" -ForegroundColor Gray
    $lines | ForEach-Object { Write-Host "  $_" }
} catch {
    Write-Host "❌ Failed: $_" -ForegroundColor Red
}

# Open browser
Write-Host ""
Write-Host "Opening browser to http://localhost:8080" -ForegroundColor Cyan
Start-Process "http://localhost:8080"

# Keep dashboard running
Write-Host "Dashboard is running. Press Ctrl+C to stop."

