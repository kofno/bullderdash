import { Queue, Worker } from 'bullmq';

// Bounded, repeatable persistence workload; index.ts remains the interactive sim.
const url = new URL(process.env.REDIS_URL || 'redis://127.0.0.1:6379');
if (!['127.0.0.1', 'localhost', '[::1]'].includes(url.hostname)) {
  throw new Error('The reliability simulator only accepts a loopback Redis URL');
}
const connection = { host: url.hostname, port: Number(url.port || 6379) };
const queueName = process.env.SIM_QUEUE || 'history-reliability';
const output = process.env.SIM_OUTPUT || 'reliability-result.json';
function positiveInt(name: string, fallback: number) {
  const value = Number(process.env[name] ?? fallback);
  if (!Number.isSafeInteger(value) || value <= 0) throw new Error(`Invalid ${name}`);
  return value;
}
const phases = [
  { name: 'steady', seconds: positiveInt('SIM_STEADY_SECONDS', 65), rate: positiveInt('SIM_STEADY_RATE', 78) },
  { name: 'burst', seconds: positiveInt('SIM_BURST_SECONDS', 15), rate: positiveInt('SIM_BURST_RATE', 2000) },
  { name: 'recovery', seconds: positiveInt('SIM_RECOVERY_SECONDS', 35), rate: positiveInt('SIM_STEADY_RATE', 78) },
];
const queue = new Queue(queueName, {
  connection,
  streams: { events: { maxLen: 1_000_000 } },
});
const client = await queue.client;
if (await client.exists(queue.toKey('id'))) {
  await queue.close();
  throw new Error(`Queue ${queueName} already exists; use a fresh isolated local Redis/queue`);
}
// Discovery must complete before the first event when the collector uses '$'.
await client.set(queue.toKey('id'), '0', 'NX');
const worker = new Worker(queueName, async job => {
  await Bun.sleep(2);
  if (job.data.sequence % 20 === 0) throw new Error('deterministic simulator failure');
  return { ok: true, sequence: job.data.sequence };
}, { connection, concurrency: 128 });
worker.on('error', error => {
  console.error(error);
  process.exitCode = 1;
});
const startedAt = new Date().toISOString();
let offered = 0;
const terminal = new Set<string>();
worker.on('completed', job => terminal.add(job.id!));
worker.on('failed', job => { if (job) terminal.add(job.id!); });
const timeline: object[] = [];
try {
  await worker.waitUntilReady();
  console.log(JSON.stringify({ event: 'ready', queue: queueName, startedAt }));
  await Bun.sleep(positiveInt('SIM_START_DELAY_SECONDS', 5) * 1000);
  for (const phase of phases) {
    const started = Date.now();
    const startOffered = offered;
    const startCounts = await queue.getJobCounts('completed', 'failed');
    // Absolute scheduling avoids accumulating timer drift between 100ms batches.
    for (let tick = 0; tick < phase.seconds * 10; tick++) {
      await Bun.sleep(Math.max(0, started + tick * 100 - Date.now()));
      const target = Math.floor((tick + 1) * phase.rate / 10);
      const jobs = [];
      while (offered - startOffered < target) {
        const sequence = offered++;
        jobs.push({
          name: 'history-reliability',
          data: {
            sequence, phase: phase.name, traceId: `simtrace-${sequence}`,
            message: 'reliabilityneedle',
            // Stable per-record tokens exercise the full-payload FTS triggers.
            payload: Array.from({ length: 96 }, (_, j) =>
              `sensor${(sequence * 97 + j * 13) % 100_003}`).join(' '),
          },
          opts: { jobId: `sim-${sequence}`, attempts: 1, removeOnComplete: false, removeOnFail: false },
        });
      }
      await queue.addBulk(jobs);
    }
    await Bun.sleep(Math.max(0, started + phase.seconds * 1000 - Date.now()));
    const counts = await queue.getJobCounts('completed', 'failed', 'waiting', 'active');
    const summary = {
      phase: phase.name, targetRate: phase.rate, elapsedMs: Date.now() - started,
      offered: offered - startOffered,
      completedDelta: counts.completed! - startCounts.completed!,
      failedDelta: counts.failed! - startCounts.failed!, counts,
    };
    timeline.push(summary);
    console.log(JSON.stringify(summary));
  }
  const deadline = Date.now() + positiveInt('SIM_DRAIN_SECONDS', 120) * 1000;
  while (terminal.size < offered) {
    if (Date.now() >= deadline) throw new Error(`Worker drain timed out: ${terminal.size}/${offered}`);
    await Bun.sleep(100);
  }
  const completedIds = await client.zrange(queue.toKey('completed'), 0, -1);
  const failedIds = await client.zrange(queue.toKey('failed'), 0, -1);
  const streamInfo = await client.xinfo('STREAM', queue.toKey('events'));
  const result = {
    queue: queueName, startedAt, endedAt: new Date().toISOString(), offered,
    timeline, completedIds, failedIds, streamInfo,
    counts: await queue.getJobCounts('completed', 'failed', 'waiting', 'active'),
    eventStreamLength: await client.xlen(queue.toKey('events')),
    eventStreamMaxLen: 1_000_000,
  };
  await Bun.write(output, JSON.stringify(result, null, 2));
  console.log(JSON.stringify({ event: 'finished', offered, counts: result.counts, output }));
} finally {
  await worker.close();
  await queue.close();
}
if (process.exitCode) throw new Error('Worker reported an error');
