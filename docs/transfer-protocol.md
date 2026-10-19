# Encrypted transfers, protocol v1

This is application-layer encryption for **passive** tunnel observers. The
HTML/JavaScript and server public key still come through the tunnel. An active
intermediary can replace them and obtain files or keys. An intermediary that
reuses observed access credentials to initiate its own authorized transfers is
also active and outside this guarantee. This protocol has not
received an independent cryptographic audit and provides no forward secrecy
against later compromise of the current process's RSA private key.

## Bootstrap and keys

All endpoints retain the instance's existing Basic Auth, when configured. No
endpoint adds wildcard CORS; cross-origin requests are refused. The feature is
opt-in through `-encrypt-transfers`; ordinary HTTP file transfers remain the
default when the flag is absent.

`GET /api/v1/crypto/key` returns JSON: `version` (1), `spki` (base64 DER RSA
public key), `fingerprint` (hex SHA-256 of that DER), `serverTime` (Unix
milliseconds), and `blockSize` (4194304). RSA uses a 3072-bit modulus. Its
private key is generated at startup, stays in memory, and changes on restart.
Fingerprint comparison within tunnel-delivered JavaScript cannot authenticate
that JavaScript.

The browser generates 64 random bytes for each transfer, independently of all
other transfers. The first 32 form the upload AES key; the remaining 32 form
the response AES key. These bytes are encapsulated with RSA-OAEP, SHA-256 for
both OAEP and MGF1, and UTF-8 label `upgopher-transfer-v1`. AES keys are imported
as non-extractable Web Crypto keys and not stored in browser storage. There is
no password-derived key and no exchange of a plaintext symmetric key.

## Authenticated packets

A packet is binary `nonce || ciphertext || tag`, with a fresh random 12-byte
nonce and AES-256-GCM's 16-byte tag. The plaintext block limit is 4 MiB. A key
pair supports fewer than one million messages per direction; frame and
completion indices enforce that bound. Probabilistic nonce uniqueness depends
on the operating system/browser cryptographic random generator.

GCM additional authenticated data is the exact UTF-8 string:

```text
upgopher-v1:<id>:<kind>:<index>
```

The ID is 32 lowercase hex characters from 16 random bytes. The decimal index
has no leading zeros. Kinds are `request-metadata`, `upload`, `finish`,
`response-metadata`, and `response-body`. Different kinds/directions cannot be
substituted. A static Web Crypto-generated interoperability vector is in
`internal/transfers/testdata/v1.json` and is checked by the Go tests.

## Request lifecycle

1. `POST /api/v1/crypto/transfers` accepts JSON `{id,key,metadata}`. `key` is the
   base64 RSA envelope; `metadata` is a base64 upload-key packet of kind
   `request-metadata`, index 0. Its plaintext is JSON `{method,url,headers,
   issuedAt}`. The URL is a same-server request target, including base64 paths
   where existing handlers require them. Only file-bearing routes and
   Content-Type/Accept/X-Requested-With headers are allowed. Authorization and
   host information come from the outer request. `issuedAt` uses `serverTime`
   from a fresh bootstrap read. Requests older than 15 minutes or more than
   30 seconds in the future are rejected. Used IDs are retained through that
   validity window to reject initiation replay. Success is HTTP 201.
2. `POST /api/v1/crypto/transfers/<id>/chunks?index=N` sends upload-key packets
   of kind `upload`, consecutively from index 0. The decrypted bytes reproduce
   the original HTTP body, including multipart boundaries. Empty bodies need
   no chunks. The server stages the complete request in a private temporary
   directory outside the shared directory, then checks its authenticated
   completion before dispatch. Upload limits count decrypted request bytes,
   including multipart overhead. Each accepted chunk returns HTTP 204.
3. `POST /api/v1/crypto/transfers/<id>/completion` sends an upload-key packet
   of kind `finish`, index equal to the uploaded chunk count. Plaintext JSON
   `{size,chunks}` must match the received total. Only then is the original
   request dispatched to the existing handler. Filesystem validation,
   read-only behavior, and atomic per-file upload finalization are retained.
4. The completion response is a response-key packet of kind
   `response-metadata`, index 0. Its plaintext JSON is `{status,headers,size,
   chunks}`; header values are arrays. All original response bytes, including
   error bodies, ZIPs, previews and search excerpts, are encrypted into private
   response staging files. Preparation completes before metadata is returned;
   staging time for large responses is subject to the tunnel's request timeout.
5. `GET /api/v1/crypto/transfers/<id>/chunks?index=N` returns response-key
   packets of kind `response-body`, indexed from 0. Retries return the original
   ciphertext. Authenticated response metadata fixes the total length and
   chunk count. The browser verifies each block before using it and rejects
   truncation/substitution. No content appears in the outer response headers.
6. `DELETE /api/v1/crypto/transfers/<id>` cancels/disposes the transfer. Normal
   response consumption disposes it automatically; idle expiry is a fallback.
   Cancellation after the application has committed a file cannot undo that
   commit. Interrupted uploads before completion are never dispatched.

There are at most 64 active transfers and 4096 retained initiation IDs in the
15-minute replay window. Resource exhaustion returns HTTP 429. Transfers idle
for 15 minutes are expired, checked on access and by a minute-based sweep.
Temporary uploads are plaintext, mode 0600, under a private mode-0700 directory;
response staging files contain ciphertext. Cancellation, expiry, and ordinary
shutdown remove them; forced termination can leave temporary files behind.

Plain file routes/uploads return HTTP 426 in encrypted mode. Browser navigation
to raw/download/custom/image links receives only an HTML loader. Names, paths,
sizes, timings, access credentials, and clipboard text remain outside the
confidentiality guarantee. Shared links need JavaScript and cannot be used as
direct image/file embeds. Unsupported browsers and failed cryptography never
fall back to plaintext. Downloads write directly to disk where supported;
otherwise the browser buffers at most 256 MiB per operation, including ZIPs.

## Validation

```sh
make dev
make ci
# Disposable instance in another terminal:
./upgopher -encrypt-transfers -dir /tmp/upgopher-crypto-test -port 19090
node scripts/test-encrypted-transfers.mjs http://localhost:19090
```

Node is only an optional development test tool, not a runtime dependency.
The smoke script uses the actual embedded JavaScript with Web Crypto and tests
multi-block files, Unicode, folders/empty files, downloads, ZIPs, previews,
search, screenshots, custom aliases, cancellation, and plaintext-route refusal.
It leaves synthetic fixtures in the selected disposable instance.
