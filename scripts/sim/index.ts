import { FlowProducer, Queue, Worker } from 'bullmq';

// We talk to 127.0.0.1:6379 because of your kind-config hostPort mapping
const connection = { host: '127.0.0.1', port: 6379 };
const queueNames = (process.env.QUEUES || 'orders,emails,billing').split(',');

// Enhanced job type configurations with more realistic patterns
const jobTypes = [
  {
    name: 'process-data',
    failRate: 0.05,        // 5% failure rate
    delayMs: 3000,         // Takes 3 seconds to process
    description: 'Data processing job'
  },
  {
    name: 'send-email',
    failRate: 0.08,        // 8% failure rate
    delayMs: 1500,         // Takes 1.5 seconds
    description: 'Email delivery'
  },
  {
    name: 'webhook-call',
    failRate: 0.12,        // 12% failure rate (external APIs!)
    delayMs: 3500,         // Takes 3.5 seconds
    description: 'Webhook notification'
  },
  {
    name: 'database-sync',
    failRate: 0.03,        // 3% failure rate
    delayMs: 2800,         // Takes 2.8 seconds
    description: 'Database synchronization'
  },
  {
    name: 'report-generate',
    failRate: 0.06,        // 6% failure rate
    delayMs: 6000,         // Takes 6 seconds (longer process)
    description: 'Generate report'
  },
  {
    name: 'order-finalize',
    failRate: 0.02,        // 2% failure rate
    delayMs: 2500,         // Takes 2.5 seconds
    description: 'Finalize order after children complete'
  },
];

type JobMix = { name: string; weight: number; priority?: number };
type QueueProfile = {
  concurrency: number;
  meanIntervalMs: number;
  burstChance: number;
  burstMultiplier: number;
  jobMix: JobMix[];
};

const defaultProfile: QueueProfile = {
  concurrency: 2,
  meanIntervalMs: 5000,
  burstChance: 0.15,
  burstMultiplier: 3,
  jobMix: [
    { name: 'process-data', weight: 30 },
    { name: 'send-email', weight: 25, priority: 2 },
    { name: 'webhook-call', weight: 20, priority: 1 },
    { name: 'database-sync', weight: 15 },
    { name: 'report-generate', weight: 10 },
  ],
};

const queueProfiles: Record<string, QueueProfile> = {
  orders: {
    ...defaultProfile,
    concurrency: 3,
    meanIntervalMs: 3500,
    jobMix: [
      { name: 'process-data', weight: 35 },
      { name: 'order-finalize', weight: 5 },
      { name: 'database-sync', weight: 25 },
      { name: 'webhook-call', weight: 20, priority: 1 },
      { name: 'send-email', weight: 15, priority: 2 },
      { name: 'report-generate', weight: 5 },
    ],
  },
  emails: {
    ...defaultProfile,
    concurrency: 2,
    meanIntervalMs: 2500,
    jobMix: [
      { name: 'send-email', weight: 60, priority: 2 },
      { name: 'webhook-call', weight: 20, priority: 1 },
      { name: 'process-data', weight: 15 },
      { name: 'report-generate', weight: 5 },
    ],
  },
  billing: {
    ...defaultProfile,
    concurrency: 1,
    meanIntervalMs: 6000,
    burstChance: 0.1,
    burstMultiplier: 2,
    jobMix: [
      { name: 'database-sync', weight: 35 },
      { name: 'process-data', weight: 30 },
      { name: 'report-generate', weight: 20 },
      { name: 'webhook-call', weight: 10, priority: 1 },
      { name: 'send-email', weight: 5, priority: 2 },
    ],
  },
};

function pickWeightedJob(mix: JobMix[]) {
  const total = mix.reduce((sum, m) => sum + m.weight, 0);
  let roll = Math.random() * total;
  for (const m of mix) {
    roll -= m.weight;
    if (roll <= 0) return m;
  }
  return mix[mix.length - 1]!;
}

function expDelay(meanMs: number) {
  return Math.max(200, Math.round(-Math.log(1 - Math.random()) * meanMs));
}

// Create workers for each queue to process jobs through states
async function setupWorkers() {
  for (const queueName of queueNames) {
    const profile = queueProfiles[queueName] ?? defaultProfile;

    // Each queue gets a worker with LOW concurrency to create backlog
    const worker = new Worker(
      queueName,
      async (job) => {
        const jobType = jobTypes.find((jt) => jt.name === job.name) ?? jobTypes[0]!;

        console.log(`  ⏳ [${queueName}] Processing job ${job.id} (${jobType.name}) - ${jobType.description}`);

        // Simulate processing time with progress updates
        const steps = 5;
        const stepDuration = jobType.delayMs / steps;

        for (let i = 1; i <= steps; i++) {
          await new Promise(resolve => setTimeout(resolve, stepDuration));
          const progress = (i / steps) * 100;
          await job.updateProgress(progress);
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
        concurrency: profile.concurrency,
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

    console.log(`🔧 Worker started for queue: ${queueName} (concurrency: ${profile.concurrency})`);
  }
}

// Continuously add new jobs to simulate realistic workload
async function addJobsContinuously() {
  const queues = new Map();
  const flowProducer = new FlowProducer({ connection });

  for (const queueName of queueNames) {
    queues.set(queueName, new Queue(queueName, { connection }));
  }

  const addJobsForQueue = async (queueName: string, queue: Queue) => {
    const profile = queueProfiles[queueName] ?? defaultProfile;
    const isBurst = Math.random() < profile.burstChance;
    const jobsToAdd = isBurst ? profile.burstMultiplier : 1;

    for (let i = 0; i < jobsToAdd; i++) {
      const picked = pickWeightedJob(profile.jobMix);
      const jobType = jobTypes.find((jt) => jt.name === picked.name) ?? jobTypes[0]!;

      // More realistic delay distribution:
      // - 70% no delay (immediate)
      // - 20% short delay (2-10 seconds)
      // - 10% longer delay (15-45 seconds)
      let delay = 0;
      const delayRoll = Math.random();
      if (delayRoll < 0.70) {
        delay = 0;
      } else if (delayRoll < 0.90) {
        delay = 2000 + Math.random() * 8000;
      } else {
        delay = 15000 + Math.random() * 30000;
      }

      const opts: any = {
        attempts: 3,
        backoff: {
          type: 'exponential',
          delay: 3000,
        },
        removeOnComplete: {
          age: 60 * 60,
        },
        removeOnFail: {
          age: 24 * 60 * 60,
        },
      };

      if (delay > 0) {
        opts.delay = delay;
      }
      if (picked.priority !== undefined) {
        opts.priority = picked.priority;
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
        const priStr = picked.priority !== undefined ? ` (p${picked.priority})` : '';
        console.log(`📤 [${queueName}] Added job ${job.id} (${jobType.name})${delayStr}${priStr}`);
      } catch (err) {
        console.error(`Failed to add job to ${queueName}:`, err);
      }
    }
  };

  // Initial batch of jobs
  console.log("\n📊 Adding initial batch of jobs...");
  for (const [queueName, queue] of queues) {
    await addJobsForQueue(queueName, queue);
  }

  // Schedule per-queue arrivals with exponential inter-arrival times
  console.log("📤 Continuing to add jobs with per-queue arrival rates...\n");
  for (const [queueName, queue] of queues) {
    const profile = queueProfiles[queueName] ?? defaultProfile;
    const scheduleNext = async () => {
      await addJobsForQueue(queueName, queue);
      setTimeout(scheduleNext, expDelay(profile.meanIntervalMs));
    };
    setTimeout(scheduleNext, expDelay(profile.meanIntervalMs));
  }

  // Periodically create parent-child flows to exercise waiting-children state
  const createFlow = async () => {
    try {
      const orderId = Math.random().toString(36).substring(2, 10);
      const parentQueue = queues.has('orders') ? 'orders' : queueNames[0]!;
      const emailQueue = queues.has('emails') ? 'emails' : parentQueue;
      const billingQueue = queues.has('billing') ? 'billing' : parentQueue;

      const flow = await flowProducer.add({
        name: 'order-finalize',
        queueName: parentQueue,
        data: {
          orderId,
          createdAt: Date.now(),
        },
        opts: {
          attempts: 2,
          removeOnComplete: { age: 60 * 60 },
          removeOnFail: { age: 24 * 60 * 60 },
        },
        children: [
          {
            name: 'process-data',
            queueName: parentQueue,
            data: { orderId, step: 'validate-items' },
            opts: { attempts: 2 },
          },
          {
            name: 'database-sync',
            queueName: billingQueue,
            data: { orderId, step: 'reserve-funds' },
            opts: { attempts: 3, priority: 1 },
          },
          {
            name: 'send-email',
            queueName: emailQueue,
            data: { orderId, step: 'send-confirmation' },
            opts: { attempts: 3, priority: 2 },
          },
        ],
      });

      console.log(`🧩 [flow] Created order flow ${flow.job.id} with children (orderId=${orderId})`);
    } catch (err) {
      console.error("Failed to create flow:", err);
    }
  };

  setInterval(createFlow, 45000 + Math.random() * 45000);
  setTimeout(createFlow, 12000);

  // Occasionally pause/resume a queue to exercise paused state
  const queuesArray = Array.from(queues.entries());
  setInterval(async () => {
    if (queuesArray.length === 0) return;
    if (Math.random() > 0.08) return;
    const [queueName, queue] = queuesArray[Math.floor(Math.random() * queuesArray.length)]!;
    try {
      console.log(`⏸️  [${queueName}] Pausing queue briefly to simulate maintenance...`);
      await queue.pause();
      setTimeout(async () => {
        try {
          await queue.resume();
          console.log(`▶️  [${queueName}] Resumed queue`);
        } catch (err) {
          console.error(`Failed to resume ${queueName}:`, err);
        }
      }, 8000 + Math.random() * 7000);
    } catch (err) {
      console.error(`Failed to pause ${queueName}:`, err);
    }
  }, 30000);
}

async function simulate() {
  console.log("🎢 Starting Bull-der-dash Realistic Job Simulator...");
  console.log(`📋 Queues: ${queueNames.join(', ')}`);
  console.log(`📊 Job types: ${jobTypes.map(jt => jt.name).join(', ')}`);
  console.log(`\n⚙️  Configuration:`);
  console.log(`  • Worker concurrency: 1 per queue (creates visible backlog)`);
  console.log(`  • Job processing time: 1.5-6 seconds each`);
  console.log(`  • New jobs added with per-queue rates + bursts`);
  console.log(`  • Failure rates: 3-12% (realistic flakiness)`);
  console.log(`  • Retry attempts: 3 per job`);
  console.log(`  • Delayed jobs: 70% immediate, 20% 2-10s delay, 10% 15-45s delay`);
  console.log(`  • Prioritized jobs: enabled on selected job types`);
  console.log(`  • Paused queues: occasional short maintenance pauses`);
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
