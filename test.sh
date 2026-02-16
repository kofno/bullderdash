#!/bin/bash

echo "🧪 Testing Bull-der-dash"
echo ""

echo "1️⃣ Testing health endpoint..."
curl -s http://localhost:8080/health
echo ""
echo ""

echo "2️⃣ Testing ready endpoint..."
curl -s http://localhost:8080/ready
echo ""
echo ""

echo "3️⃣ Testing queues endpoint..."
curl -s http://localhost:8080/queues | head -50
echo ""
echo ""

echo "4️⃣ Testing metrics endpoint..."
curl -s http://localhost:8080/metrics | grep bullmq | head -5
echo ""

