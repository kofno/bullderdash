import { Queue, Worker } from 'bullmq';

// We talk to 127.0.0.1:6379 because of your kind-config hostPort mapping
const connection = { host: '127.0.0.1', port: 6379 };
const queueNames = (process.env.QUEUES || 'orders,emails,billing').split(',');

// Configuration for different job types with different outcomes
const jobTypes = [
  { name: 'process-data', failRate: 0.1, delayMs: 1000 },      // 90% success rate
  { name: 'send-email', failRate: 0.2, delayMs: 500 },          // 80% success rate
  { name: 'webhook-call', failRate: 0.3, delayMs: 800 },        // 70% success rate
  { name: 'database-sync', failRate: 0.05, delayMs: 2000 },     // 95% success rate
];

// Create workers for each queue to process jobs through states
async function setupWorkers() {
  for (const queueName of queueNames) {
    // Each queue gets a worker that simulates processing
    const worker = new Worker(
      queueName,
      async (job) => {
        const jobType = jobTypes[Math.floor(Math.random() * jobTypes.length)];

        // Simulate processing time
        await new Promise(resolve => setTimeout(resolve, jobType.delayMs));

        // Update progress
        job.updateProgress(50);
        await new Promise(resolve => setTimeout(resolve, 100));

        // Randomly fail some jobs (for testing failure states)
        if (Math.random() < jobType.failRate) {
          job.updateProgress(75);
          throw new Error(`Simulated failure in ${job.name}: ${['Network timeout', 'Invalid data', 'Resource not found'][Math.floor(Math.random() * 3)]}`);
        }

        job.updateProgress(100);
        return {
          success: true,
          processedAt: new Date().toISOString(),
          jobName: job.name,
          queueName: queueName
        };
      },
      {
        connection,
        concurrency: 3,  // Process up to 3 jobs concurrently
      }
    );

    worker.on('completed', (job, result) => {
      console.log(`  ✅ [${queueName}] Job ${job.id} (${job.name}) completed`);
    });

    worker.on('failed', (job, err) => {
      console.log(`  ❌ [${queueName}] Job ${job.id} (${job.name}) failed: ${err.message}`);
    });

    console.log(`🔧 Worker started for queue: ${queueName}`);
  }
}

// Continuously add new jobs to simulate realistic workload
async function addJobsContinuously() {
  const queues = new Map();

  for (const queueName of queueNames) {
    queues.set(queueName, new Queue(queueName, { connection }));
  }

  // Add jobs every 2-4 seconds
  const addJobs = async () => {
    for (const [queueName, queue] of queues) {
      const jobType = jobTypes[Math.floor(Math.random() * jobTypes.length)];

      // Randomly add delayed jobs (20% chance)
      const delay = Math.random() < 0.2 ? Math.random() * 10000 : 0;

      // Add some jobs with retry options to see them move through states
      const opts: any = {
        attempts: 3,
        backoff: {
          type: 'exponential',
          delay: 2000,
        },
      };

      if (delay > 0) {
        opts.delay = delay;
      }

      try {
        const job = await queue.add(
          jobType.name,
          {
            id: Math.random().toString(36).substring(7),
            timestamp: Date.now(),
            data: {
              value: Math.floor(Math.random() * 1000),
              user: `user-${Math.floor(Math.random() * 100)}`,
            },
          },
          opts
        );
        console.log(`📤 [${queueName}] Added job ${job.id} (${jobType.name})${delay > 0 ? ` (delayed ${(delay / 1000).toFixed(1)}s)` : ''}`);
      } catch (err) {
        console.error(`Failed to add job to ${queueName}:`, err);
      }
    }
  };

  // Initial jobs
  await addJobs();

  // Keep adding jobs continuously
  setInterval(addJobs, 2000 + Math.random() * 2000);
}

async function simulate() {
  console.log("🎢 Starting Bull-der-dash Enhanced Job Simulator...");
  console.log(`📋 Queues: ${queueNames.join(', ')}`);
  console.log(`📊 Job types: ${jobTypes.map(jt => jt.name).join(', ')}`);
  console.log("");

  try {
    // Setup workers first
    await setupWorkers();
    console.log("");

    // Then start adding jobs
    await addJobsContinuously();

    console.log("🚀 Simulator running... Press Ctrl+C to stop\n");

    // Keep the process alive
    // Job states will flow through: waiting → active → completed/failed
    // Some will be retried, some will be delayed
  } catch (err) {
    console.error("Simulation error:", err);
    process.exit(1);
  }
}

// Graceful shutdown
process.on('SIGINT', async () => {
  console.log("\n📊 Simulator shutting down gracefully...");
  process.exit(0);
});

simulate().catch(console.error);