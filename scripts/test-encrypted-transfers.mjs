// Run against a disposable encrypted Upgopher instance:
// node scripts/test-encrypted-transfers.mjs http://localhost:19090
// Optional Basic Auth: UPGOPHER_TEST_AUTH='test:password' node ...
// No browser packages or production runtime dependencies are needed.
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { webcrypto } from 'node:crypto';
import vm from 'node:vm';

const origin = new URL(process.argv[2] || 'http://localhost:19090').origin;
const marker = 'UPGOPHER_PRIVATE_SMOKE_CONTENT_48fa7d';
const auth = process.env.UPGOPHER_TEST_AUTH;
let encryptedPackets = 0;
async function wireFetch(input, options = {}) {
    const request = new Request(new URL(input, origin), options);
    if (auth) request.headers.set('Authorization', 'Basic ' + Buffer.from(auth).toString('base64'));
    if (request.body && new URL(request.url).pathname.startsWith('/api/v1/crypto/')) {
        const sent = Buffer.from(await request.clone().arrayBuffer());
        assert.equal(sent.includes(Buffer.from(marker)), false, 'plaintext upload crossed the transport');
    }
    const response = await fetch(request);
    if (new URL(request.url).pathname.includes('/chunks')) {
        encryptedPackets++;
        const received = Buffer.from(await response.clone().arrayBuffer());
        assert.equal(received.includes(Buffer.from(marker)), false, 'plaintext download crossed the transport');
    }
    return response;
}
const window = { fetch: wireFetch, location: { href: origin + '/', origin }, isSecureContext: true, crypto: webcrypto };
const context = vm.createContext({ window, crypto: webcrypto, TextEncoder, TextDecoder, Request, Response, Headers, URL, ReadableStream, Uint8Array, DOMException, AbortController, setTimeout,
    btoa: text => Buffer.from(text, 'binary').toString('base64'), atob: text => Buffer.from(text, 'base64').toString('binary'),
    document: { addEventListener() {} } });
vm.runInContext(await readFile(new URL('../internal/statics/js/crypto.js', import.meta.url), 'utf8'), context);
const client = window.UpgopherCrypto;
const name = 'crypto-smoke-' + Date.now();
const payload = Buffer.alloc(8 * 1024 * 1024 + 257);
for (let i = 0; i < payload.length; i++) payload[i] = marker.charCodeAt(i % marker.length);
const form = new FormData();
form.append('file', new Blob([payload]), name + '/large.bin');
form.append('file', new Blob([marker]), name + '/á-small.txt');
form.append('file', new Blob([]), name + '/empty.txt');
form.append('empty-dir', new Blob([]), name + '/empty-folder');
let response = await client.fetch('/', { method: 'POST', headers: { 'X-Requested-With': 'XMLHttpRequest' }, body: form });
assert.equal(response.status, 201, await response.clone().text());
const uploaded = await response.json();
assert.equal(uploaded.files, 3);
assert.equal(uploaded.dirs, 1);
const path = relative => encodeURIComponent(Buffer.from(name + '/' + relative).toString('base64'));
response = await client.fetch('/download/?path=' + path('large.bin'));
assert.equal(response.status, 200);
assert.deepEqual(Buffer.from(await response.arrayBuffer()), payload);
response = await client.fetch('/download/?path=' + path('empty.txt'));
assert.equal((await response.arrayBuffer()).byteLength, 0);
response = await client.fetch('/file-content?path=' + path('á-small.txt'));
assert.equal((await response.json()).content, marker);
response = await client.fetch('/search-file?path=' + path('á-small.txt') + '&term=PRIVATE');
assert.ok(JSON.stringify(await response.json()).includes(marker));
response = await client.fetch('/zip-selected', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ paths: [Buffer.from(name).toString('base64')] }) });
const zip = Buffer.from(await response.arrayBuffer());
assert.equal(zip.readUInt32LE(0), 0x04034b50);
const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jB9sAAAAASUVORK5CYII=', 'base64');
response = await client.fetch('/api/v1/screenshots', { method: 'POST', headers: { 'Content-Type': 'image/png' }, body: new Blob([png]) });
assert.equal(response.status, 201);
const screenshot = await response.json();
response = await client.fetch('/api/v1/screenshots/image?id=' + screenshot.id);
assert.deepEqual(Buffer.from(await response.arrayBuffer()), png);
for (const target of ['/download/?path=' + path('large.bin'), '/raw/' + name + '/large.bin', '/file-content?path=' + path('á-small.txt'), '/zip', '/zip-selected', '/search-file?path=' + path('á-small.txt'), '/api/v1/screenshots/image?id=' + screenshot.id, '/screenshot/' + screenshot.id]) {
    response = await wireFetch(target);
    assert.equal(response.status, 426, 'plaintext route accepted: ' + target);
    assert.equal((await response.text()).includes(marker), false);
}
// Custom aliases must also be encrypted; creation must not follow a redirect
// into a plaintext file download.
response = await wireFetch('/custom-path', { method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded',Accept:'application/json'},body:new URLSearchParams({originalPath:name+'/large.bin',customPath:name+'-alias'}) });
assert.equal(response.status,201);
await response.json();
response = await client.fetch('/'+name+'-alias');
assert.deepEqual(Buffer.from(await response.arrayBuffer()),payload);
response = await wireFetch('/'+name+'-alias');
assert.equal(response.status,426);
// Shared URLs should load a shell, never the original bytes.
response = await wireFetch('/raw/' + name + '/large.bin', { headers: { Accept: 'text/html' } });
assert.equal(response.status, 200);
assert.equal((await response.text()).includes(marker), false);
// A cancelled request never reaches the underlying multipart upload handler.
const abort = new AbortController();
const cancelled = new FormData();
cancelled.append('file', new Blob([payload]), name + '/cancelled.bin');
await assert.rejects(client.fetch('/', { method: 'POST', body: cancelled, signal: abort.signal, onProgress() { abort.abort(); } }));
response = await client.fetch('/download/?path=' + path('cancelled.bin'));
assert.equal(response.status, 404); await response.text();
assert.ok(encryptedPackets >= 10);
console.log('PASS: Web Crypto ↔ Go; multi-block uploads/downloads, Unicode, empty files/folders, previews, search, ZIP, screenshots, cancellation, and plaintext route blocking (' + encryptedPackets + ' packets).');
console.log('Test fixtures remain in the disposable directory: ' + name);
