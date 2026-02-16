import { Queue } from 'bullmq';

// We talk to 127.0.0.1:6379 because of your kind-config hostPort mapping
const connection = { host: '127.0.0.1', port: 6379 };
const queueNames = (process.env.QUEUES || 'orders,emails,billing').split(',');

async function simulate() {
  console.log("🎢 Starting Bull-der-dash Job Simulator...");

  for (const name of queueNames) {
    const queue = new Queue(name, { connection });

    // Add a "healthy" job
    await queue.add('process-data', { foo: 'bar', timestamp: Date.now() });

    // Add a job that we'll pretend failed
    await queue.add('critical-alert', { error: 'Simulated failure' });

    // In a real app, a worker would fail this, but for now, we're just
    // ensuring the keyspace (bull:queue:wait, etc.) exists.
    console.log(` ✅ Populated queue: ${name}`);
  }

  console.log("🚀 Simulation complete. Valkey is now dirty.");
  process.exit(0);
}

simulate().catch(console.error);