import { Queue, Worker } from 'bullmq';

// We talk to 127.0.0.1:6379 because of your kind-config hostPort mapping
const connection = { host: '127.0.0.1', port: 6379 };
const queueNames = (process.env.QUEUES || 'orders,emails,billing').split(',');

// Enhanced job type configurations with more realistic patterns
const jobTypes = [
  {
    name: 'process-data',
    failRate: 0.15,        // 15% failure rate
    delayMs: 3000,         // Takes 3 seconds to process
    description: 'Data processing job'
  },
  {
    name: 'send-email',
    failRate: 0.25,        // 25% failure rate (emails are flaky!)
    delayMs: 2000,         // Takes 2 seconds
    description: 'Email delivery'
  },
  {
    name: 'webhook-call',
    failRate: 0.35,        // 35% failure rate (external APIs!)
    delayMs: 4000,         // Takes 4 seconds
    description: 'Webhook notification'
  },
  {
    name: 'database-sync',
    failRate: 0.10,        // 10% failure rate
    delayMs: 2500,         // Takes 2.5 seconds
    description: 'Database synchronization'
  },
  {
    name: 'report-generate',
    failRate: 0.20,        // 20% failure rate
    delayMs: 5000,         // Takes 5 seconds (longer process)
    description: 'Generate report'
  },
];

// Create workers for each queue to process jobs through states
async function setupWorkers() {
  for (const queueName of queueNames) {
    // Each queue gets a worker with LOW concurrency to create backlog
    const worker = new Worker(
      queueName,
      async (job) => {
        const jobType = jobTypes[Math.floor(Math.random() * jobTypes.length)]!;

        console.log(`  ⏳ [${queueName}] Processing job ${job.id} (${jobType.name}) - ${jobType.description}`);

        // Simulate processing time with progress updates
        const steps = 5;
        const stepDuration = jobType.delayMs / steps;

        for (let i = 1; i <= steps; i++) {
          await new Promise(resolve => setTimeout(resolve, stepDuration));
          const progress = (i / steps) * 100;
          job.updateProgress(progress);
        }

        // Randomly fail some jobs (for testing failure states)
        if (Math.random() < jobType.failRate) {
          const errors = [
            'Network timeout',
            'Invalid data format',
            'Resource not found',
            'Permission denied',
            'Service unavailable',
            'Database connection lost',
          ];
          const error = errors[Math.floor(Math.random() * errors.length)];
          console.log(`  ❌ [${queueName}] Job ${job.id} failed: ${error}`);
          throw new Error(`${jobType.name} failed: ${error}`);
        }

        console.log(`  ✅ [${queueName}] Job ${job.id} (${jobType.name}) completed`);
        return {
          success: true,
          processedAt: new Date().toISOString(),
          jobName: job.name,
          queueName: queueName,
          processingTime: jobType.delayMs
        };
      },
      {
        connection,
        concurrency: 1,  // Process only 1 job at a time to create visible queue
        // Enable auto-removal to clean up completed jobs after a delay
        autorun: true,
      }
    );

    worker.on('completed', (job, _result) => {
      console.log(`  ✅ [${queueName}] Job ${job?.id} completed successfully`);
    });

    worker.on('failed', (job, err) => {
      console.log(`  ❌ [${queueName}] Job ${job?.id} failed after ${job?.attemptsMade} attempt(s): ${err.message}`);
    });

    worker.on('error', (err) => {
      console.error(`  💥 [${queueName}] Worker error:`, err.message);
    });

    console.log(`🔧 Worker started for queue: ${queueName} (concurrency: 1 - creates visible backlog)`);
  }
}

// Continuously add new jobs to simulate realistic workload
async function addJobsContinuously() {
  const queues = new Map();

  for (const queueName of queueNames) {
    queues.set(queueName, new Queue(queueName, { connection }));
  }

  // Add jobs with variable timing to create realistic patterns
  const addJobs = async () => {
    for (const [queueName, queue] of queues) {
      // Randomly choose 1-3 jobs to add each cycle
      const jobsToAdd = Math.floor(Math.random() * 3) + 1;

      for (let i = 0; i < jobsToAdd; i++) {
        const jobType = jobTypes[Math.floor(Math.random() * jobTypes.length)]!;

        // More realistic delay distribution:
        // - 60% no delay (immediate)
        // - 30% short delay (5-15 seconds)
        // - 10% longer delay (30-60 seconds)
        let delay = 0;
        const delayRoll = Math.random();
        if (delayRoll < 0.60) {
          delay = 0;  // Execute immediately
        } else if (delayRoll < 0.90) {
          delay = 5000 + Math.random() * 10000;  // 5-15 seconds
        } else {
          delay = 30000 + Math.random() * 30000;  // 30-60 seconds
        }

        const opts: any = {
          attempts: 3,  // Allow 3 attempts before giving up
          backoff: {
            type: 'exponential',
            delay: 3000,  // 3 second base delay between retries
          },
          removeOnComplete: {
            age: 60 * 60,  // Keep completed jobs for 1 hour
          },
          removeOnFail: {
            age: 24 * 60 * 60,  // Keep failed jobs for 24 hours
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
                operation: jobType.description,
              },
            },
            opts
          );

          const delayStr = delay > 0 ? ` (delayed ${(delay / 1000).toFixed(1)}s)` : '';
          console.log(`📤 [${queueName}] Added job ${job.id} (${jobType.name})${delayStr}`);
        } catch (err) {
          console.error(`Failed to add job to ${queueName}:`, err);
        }
      }
    }
  };

  // Initial batch of jobs
  console.log("\n📊 Adding initial batch of jobs...");
  await addJobs();

  // Add jobs every 8-12 seconds to keep a visible queue
  // (since each job takes 2-5 seconds and we process 1 at a time,
  // the queue will fill up and drain, creating realistic workflow)
  console.log("📤 Continuing to add jobs every 8-12 seconds...\n");
  setInterval(addJobs, 8000 + Math.random() * 4000);
}

async function simulate() {
  console.log("🎢 Starting Bull-der-dash Realistic Job Simulator...");
  console.log(`📋 Queues: ${queueNames.join(', ')}`);
  console.log(`📊 Job types: ${jobTypes.map(jt => jt.name).join(', ')}`);
  console.log(`\n⚙️  Configuration:`);
  console.log(`  • Worker concurrency: 1 per queue (creates visible backlog)`);
  console.log(`  • Job processing time: 2-5 seconds each`);
  console.log(`  • New jobs added every 8-12 seconds`);
  console.log(`  • Failure rates: 10-35% (realistic flakiness)`);
  console.log(`  • Retry attempts: 3 per job`);
  console.log(`  • Delayed jobs: 60% immediate, 30% 5-15s delay, 10% 30-60s delay`);
  console.log("\n📈 What you'll see:");
  console.log(`  ✅ Jobs in WAITING state (backlog building up)`);
  console.log(`  🚀 Jobs in ACTIVE state (currently processing)`);
  console.log(`  ❌ Jobs in FAILED state (after 3 retry attempts)`);
  console.log(`  ✅ Jobs in COMPLETED state (before cleanup)`);
  console.log(`  ⏰ Jobs in DELAYED state (scheduled for later)`);
  console.log("\n");

  try {
    // Setup workers first
    await setupWorkers();
    console.log("");

    // Then start adding jobs
    await addJobsContinuously();

    console.log("🚀 Simulator running... Press Ctrl+C to stop\n");
    console.log("💡 Tip: Run 'QUEUE-STATS orders' in redis-cli to see jobs in different states!");
    console.log("💡 Tip: Visit http://localhost:8080 to view the dashboard\n");

    // Keep the process alive
  } catch (err) {
    console.error("Simulation error:", err);
    process.exit(1);
  }
}

// Graceful shutdown
process.on('SIGINT', async () => {
  console.log("\n\n📊 Simulator shutting down gracefully...");
  console.log("(Jobs in progress will continue processing)");
  process.exit(0);
});

simulate().catch(console.error);