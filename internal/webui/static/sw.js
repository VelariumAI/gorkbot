importScripts('https://storage.googleapis.com/workbox-cdn/releases/6.5.4/workbox-sw.js');

if (workbox) {
  // Cache static assets (CSS, JS, Fonts)
  workbox.routing.registerRoute(
    ({request}) => request.destination === 'style' || request.destination === 'script' || request.destination === 'font',
    new workbox.strategies.CacheFirst({
      cacheName: 'static-resources',
      plugins: [
        new workbox.expiration.ExpirationPlugin({
          maxEntries: 50,
          maxAgeSeconds: 30 * 24 * 60 * 60, // 30 Days
        }),
      ],
    })
  );

  // Dynamic API calls (Network First)
  workbox.routing.registerRoute(
    ({url}) => url.pathname.startsWith('/api/') && !url.pathname.includes('/stream'),
    new workbox.strategies.NetworkFirst({
      cacheName: 'api-responses',
      plugins: [
        new workbox.expiration.ExpirationPlugin({
          maxEntries: 100,
          maxAgeSeconds: 24 * 60 * 60, // 24 Hours
        }),
      ],
    })
  );
}

// Background Sync Logic
// Since we cannot easily import Dexie mjs into standard sw without a bundler,
// we will intercept the sync event and message the client to perform the sync.
self.addEventListener('sync', (event) => {
  if (event.tag === 'gorkbot-sync') {
    event.waitUntil(triggerClientSync());
  }
});

async function triggerClientSync() {
  const clients = await self.clients.matchAll();
  for (const client of clients) {
    client.postMessage({ type: 'TRIGGER_SYNC' });
  }
}
