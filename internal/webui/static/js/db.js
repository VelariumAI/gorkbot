import Dexie from 'https://cdn.jsdelivr.net/npm/dexie@3.2.4/dist/dexie.mjs';

export const db = new Dexie('GorkbotDB');

db.version(1).stores({
  offlineQueue: '++id, payload, status, timestamp, retryCount', // status: 'queued', 'syncing', 'failed'
  toolRegistry: 'name, description, category, schema',
  usageAnalytics: '++id, action, timestamp, duration, batteryLevel'
});

export class DatabaseManager {
  static async queueTask(payload) {
    return await db.offlineQueue.add({
      payload,
      status: 'queued',
      timestamp: Date.now(),
      retryCount: 0
    });
  }

  static async getPendingTasks() {
    return await db.offlineQueue.where('status').equals('queued').toArray();
  }

  static async updateTaskStatus(id, status, retryCount = 0) {
    await db.offlineQueue.update(id, { status, retryCount });
  }

  static async removeTask(id) {
    await db.offlineQueue.delete(id);
  }
  
  static async logUsage(action, duration) {
    let battery = null;
    if (navigator.getBattery) {
      try {
        const b = await navigator.getBattery();
        battery = b.level;
      } catch (e) {
        console.warn("Battery API error", e);
      }
    }
    await db.usageAnalytics.add({
      action, timestamp: Date.now(), duration, batteryLevel: battery
    });
  }
}
