/* Application-layer encryption. This script is delivered by the tunnel, so it
 * cannot protect against an intermediary that replaces the script itself. */
(function () {
    'use strict';
    const nativeFetch = window.fetch.bind(window);
    const blockSize = 4 * 1024 * 1024;
    const bufferLimit = 256 * 1024 * 1024;
    const endpoint = '/api/v1/crypto/transfers';
    const utf8 = new TextEncoder();
    const decoder = new TextDecoder();
    function available() {
        if (!window.isSecureContext || !window.crypto || !window.crypto.subtle) {
            throw new Error('Encrypted transfers require HTTPS or localhost and a browser with Web Crypto. No plaintext fallback is allowed.');
        }
    }
    function b64(bytes) {
        let text = '';
        for (let i = 0; i < bytes.length; i += 8192) text += String.fromCharCode.apply(null, bytes.subarray(i, i + 8192));
        return btoa(text);
    }
    function unb64(text) { return Uint8Array.from(atob(text), c => c.charCodeAt(0)); }
    function aad(id, kind, index) { return utf8.encode('upgopher-v1:' + id + ':' + kind + ':' + index); }
    async function seal(key, id, kind, index, bytes) {
        if (index >= 1000000) throw new Error('Transfer exceeds the encrypted message limit.');
        const iv = crypto.getRandomValues(new Uint8Array(12));
        const ciphertext = await crypto.subtle.encrypt({ name: 'AES-GCM', iv: iv, additionalData: aad(id, kind, index), tagLength: 128 }, key, bytes);
        const packet = new Uint8Array(12 + ciphertext.byteLength);
        packet.set(iv); packet.set(new Uint8Array(ciphertext), 12);
        return packet;
    }
    async function open(key, id, kind, index, packet) {
        if (packet.byteLength < 28) throw new Error('Incomplete encrypted response.');
        const bytes = new Uint8Array(packet);
        return new Uint8Array(await crypto.subtle.decrypt({ name: 'AES-GCM', iv: bytes.subarray(0, 12), additionalData: aad(id, kind, index), tagLength: 128 }, key, bytes.subarray(12)));
    }
    async function checked(url, options) {
        const response = await nativeFetch(url, Object.assign({ credentials: 'same-origin', cache: 'no-store' }, options));
        if (!response.ok) throw new Error('Encrypted transfer failed (HTTP ' + response.status + ').');
        return response;
    }
    async function serverKey() {
        available();
        const info = await (await checked('/api/v1/crypto/key')).json();
        if (info.version !== 1 || info.blockSize !== blockSize) throw new Error('Unsupported encryption protocol.');
        const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', unb64(info.spki)));
        const fingerprint = Array.from(digest, x => x.toString(16).padStart(2, '0')).join('');
        if (fingerprint !== info.fingerprint) throw new Error('Invalid server key fingerprint.');
        return info;
    }
    function sensitive(path, method) {
        if (path === '/') return method !== 'GET' && method !== 'HEAD';
        if (['/zip', '/zip-selected', '/file-content', '/search-file', '/api/v1/screenshots/image'].includes(path)) return true;
        if (path === '/api/v1/screenshots') return method === 'POST';
        if (['/api/v1/breadcrumbs', '/api/v1/tree', '/clipboard', '/clipboard/tabs', '/clipboard/stream', '/mkdir', '/custom-path', '/showhiddenfiles', '/favicon.ico', '/static/logopher.webp'].includes(path)) return false;
        return !path.startsWith('/api/v1/crypto/') && !path.startsWith('/delete/');
    }
    async function encryptedFetch(input, options, allowLarge) {
        available();
        options = options || {};
        const onProgress = options.onProgress;
        const request = new Request(input instanceof Request ? input : new URL(input, window.location.href), options);
        const url = new URL(request.url);
        if (url.origin !== window.location.origin) throw new Error('Encrypted transfers must use this server.');
        const info = await serverKey();
        const publicKey = await crypto.subtle.importKey('spki', unb64(info.spki), { name: 'RSA-OAEP', hash: 'SHA-256' }, false, ['encrypt']);
        const raw = crypto.getRandomValues(new Uint8Array(64));
        const uploadKey = await crypto.subtle.importKey('raw', raw.subarray(0, 32), 'AES-GCM', false, ['encrypt']);
        const downloadKey = await crypto.subtle.importKey('raw', raw.subarray(32), 'AES-GCM', false, ['decrypt']);
        const wrapped = await crypto.subtle.encrypt({ name: 'RSA-OAEP', label: utf8.encode('upgopher-transfer-v1') }, publicKey, raw);
        raw.fill(0);
        const id = Array.from(crypto.getRandomValues(new Uint8Array(16)), x => x.toString(16).padStart(2, '0')).join('');
        const headers = {};
        ['Content-Type', 'Accept', 'X-Requested-With'].forEach(name => { const value = request.headers.get(name); if (value) headers[name] = value; });
        const metadata = await seal(uploadKey, id, 'request-metadata', 0, utf8.encode(JSON.stringify({ method: request.method, url: url.pathname + url.search, headers: headers, issuedAt: info.serverTime })));
        const transferURL = endpoint + '/' + id;
        let created = false;
        let disposed = false;
        const cleanup = async function () {
            if (!created || disposed) return;
            disposed = true;
            try { await nativeFetch(transferURL, { method: 'DELETE', credentials: 'same-origin', cache: 'no-store', keepalive: true }); } catch (_) { /* Server also expires abandoned transfers. */ }
        };
        try {
            await checked(endpoint, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ id: id, key: b64(new Uint8Array(wrapped)), metadata: b64(metadata) }), signal: request.signal });
            created = true;
            let count = 0, total = 0;
            if (request.body) {
                const reader = request.body.getReader();
                let buffer = new Uint8Array(blockSize), used = 0;
                const send = async function (bytes) {
                    const packet = await seal(uploadKey, id, 'upload', count, bytes);
                    await checked(transferURL + '/chunks?index=' + count, { method: 'POST', headers: { 'Content-Type': 'application/octet-stream' }, body: packet, signal: request.signal });
                    total += bytes.byteLength; count++;
                    if (onProgress) onProgress(total);
                };
                try {
                    while (true) {
                        const piece = await reader.read();
                        if (piece.done) break;
                        let offset = 0;
                        while (offset < piece.value.byteLength) {
                            const length = Math.min(blockSize - used, piece.value.byteLength - offset);
                            buffer.set(piece.value.subarray(offset, offset + length), used);
                            used += length; offset += length;
                            if (used === blockSize) { await send(buffer); buffer = new Uint8Array(blockSize); used = 0; }
                        }
                    }
                    if (used) await send(buffer.subarray(0, used));
                } finally { await reader.cancel().catch(function () {}); }
            }
            const finish = await seal(uploadKey, id, 'finish', count, utf8.encode(JSON.stringify({ size: total, chunks: count })));
            const answer = await checked(transferURL + '/completion', { method: 'POST', headers: { 'Content-Type': 'application/octet-stream' }, body: finish, signal: request.signal });
            const meta = JSON.parse(decoder.decode(await open(downloadKey, id, 'response-metadata', 0, await answer.arrayBuffer())));
            if (!Number.isSafeInteger(meta.size) || meta.size < 0 || !Number.isSafeInteger(meta.chunks) || meta.chunks < 0 || meta.chunks > 1000000 || meta.chunks !== Math.ceil(meta.size / blockSize)) throw new Error('Invalid encrypted response length.');
            if (!allowLarge && meta.size > bufferLimit) throw new Error('This browser operation supports encrypted downloads up to 256 MiB. Use the Download button in a browser that supports saving directly to disk.');
            const responseHeaders = new Headers();
            Object.keys(meta.headers).forEach(name => meta.headers[name].forEach(value => responseHeaders.append(name, value)));
            responseHeaders.delete('Content-Length'); responseHeaders.delete('Content-Encoding'); responseHeaders.delete('Transfer-Encoding');
            if ([204, 205, 304].includes(meta.status)) { await cleanup(); return new Response(null, { status: meta.status, headers: responseHeaders }); }
            let index = 0, received = 0;
            const stream = new ReadableStream({
                async pull(controller) {
                    try {
                        if (request.signal.aborted) throw new DOMException('Transfer cancelled', 'AbortError');
                        if (index === meta.chunks) {
                            if (received !== meta.size) throw new Error('Incomplete encrypted file.');
                            await cleanup(); controller.close(); return;
                        }
                        const response = await checked(transferURL + '/chunks?index=' + index, { signal: request.signal });
                        const bytes = await open(downloadKey, id, 'response-body', index, await response.arrayBuffer());
                        const expected = Math.min(blockSize, meta.size - received);
                        if (bytes.byteLength !== expected) throw new Error('Invalid encrypted block length.');
                        index++; received += bytes.byteLength; controller.enqueue(bytes);
                    } catch (error) { await cleanup(); controller.error(error); }
                }, async cancel() { await cleanup(); }
            });
            return new Response(stream, { status: meta.status, headers: responseHeaders });
        } catch (error) { await cleanup(); throw error; }
    }
    function defaultName(url) {
        const parsed = new URL(url, window.location.href);
        if (parsed.pathname.includes('zip')) return parsed.pathname === '/zip-selected' ? 'selected-files.zip' : 'files.zip';
        const encoded = parsed.searchParams.get('path');
        if (encoded) { try { return decoder.decode(Uint8Array.from(atob(encoded), c => c.charCodeAt(0))).split(/[\\/]/).pop() || 'download'; } catch (_) {} }
        return decodeURIComponent(parsed.pathname.split('/').pop() || 'download');
    }
    async function download(url, options) {
        available();
        let handle;
        // Invoke the picker before awaiting network or crypto to preserve the
        // browser's user activation requirement. Other browsers use a Blob.
        if (typeof window.showSaveFilePicker === 'function') handle = await window.showSaveFilePicker({ suggestedName: defaultName(url) });
        const response = await encryptedFetch(url, options, !!handle);
        if (!response.ok) { await response.body.cancel(); throw new Error('Download failed (HTTP ' + response.status + ').'); }
        if (handle) {
            const writable = await handle.createWritable();
            try { await response.body.pipeTo(writable); } catch (error) { await writable.abort().catch(function () {}); throw error; }
        } else {
            const blob = await response.blob();
            const objectURL = URL.createObjectURL(blob);
            const link = document.createElement('a');
            link.href = objectURL; link.download = defaultName(url); document.body.appendChild(link); link.click(); link.remove();
            setTimeout(function () { URL.revokeObjectURL(objectURL); }, 60000);
        }
    }
    window.UpgopherCrypto = {
        fetch: encryptedFetch, download: download,
        async showFingerprint() {
            try { const info = await serverKey(); alert('Server SHA-256 fingerprint:\n' + info.fingerprint + '\n\nCompare it through a trusted channel. A modified page can falsify this check.'); }
            catch (error) { alert(error.message); }
        },
        async upload(form, url) {
            const controller = new AbortController();
            const cancel = document.createElement('button');
            cancel.type = 'button'; cancel.textContent = 'Cancel upload'; cancel.className = 'btn'; cancel.onclick = function () { controller.abort(); };
            const progress = document.getElementById('upload-progress-container');
            if (progress) progress.appendChild(cancel);
            try {
                const response = await encryptedFetch(url, { method: 'POST', headers: { 'X-Requested-With': 'XMLHttpRequest' }, body: form, signal: controller.signal,
                    onProgress: function (bytes) { const loaded = Math.min(bytes, uploadTotalSize); updateUploadProgress(loaded, uploadTotalSize, uploadTotalSize ? loaded / uploadTotalSize * 100 : 100); } });
                if (!response.ok) throw new Error((await response.text()).trim() || 'Upload failed.');
                await response.text(); // Drain and dispose the encrypted response.
                updateUploadProgress(uploadTotalSize, uploadTotalSize, 100);
                setTimeout(function () { window.location.reload(); }, 500);
            } catch (error) { hideUploadProgress(); alert(error.name === 'AbortError' ? 'Upload cancelled.' : error.message); }
            finally { cancel.remove(); }
        }
    };
    window.fetch = function (input, options) {
        const url = new URL(input instanceof Request ? input.url : input, window.location.href);
        const method = ((options && options.method) || (input instanceof Request && input.method) || 'GET').toUpperCase();
        if (url.origin === window.location.origin && sensitive(url.pathname, method)) return encryptedFetch(input, options);
        return nativeFetch(input, options);
    };
    document.addEventListener('DOMContentLoaded', function () {
        try { available(); }
        catch (error) {
            const status = document.getElementById('crypto-status') || document.getElementById('status');
            if (status) status.textContent = error.message;
            document.querySelectorAll('input[type=file], #upload-btn, #receive').forEach(element => { element.disabled = true; });
        }
    });
})();
