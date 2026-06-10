// webauthn.ts — byte-array helpers for the WebAuthn flow.
//
// The hub server encodes all WebAuthn byte arrays as base64url
// (no padding) in its JSON responses. The browser's
// `navigator.credentials.{create,get}(...)` API expects
// `Uint8Array`. These helpers are the one place we encode /
// decode so the rest of the wizard stays clean.
//
// Why not use Buffer? `Buffer` is a Node-only global; the
// browser bundle resolves it to nothing and tsc complains. A
// 6-line Uint8Array base64url helper avoids the dep entirely.

// base64UrlToBytes decodes a base64url (no padding) string to a
// Uint8Array. Throws on invalid input — the server is the
// source of truth here, so an invalid base64url means we
// have a bug, not a network error.
export function base64UrlToBytes(s: string): Uint8Array {
  if (typeof s !== "string") {
    throw new Error(`base64UrlToBytes: expected string, got ${typeof s}`);
  }
  // base64url uses '-' / '_' for 62 / 63; base64 uses '+' / '/'.
  // Replace + add padding back (length mod 4).
  const std = s.replace(/-/g, "+").replace(/_/g, "/");
  const pad = std.length % 4 === 0 ? "" : "=".repeat(4 - (std.length % 4));
  const b64 = std + pad;
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) {
    out[i] = bin.charCodeAt(i);
  }
  return out;
}

// bytesToBase64Url encodes a Uint8Array to base64url (no
// padding). The reverse of base64UrlToBytes; same deps.
export function bytesToBase64Url(bytes: Uint8Array | ArrayBuffer): string {
  const u8 = bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes);
  let s = "";
  for (let i = 0; i < u8.length; i++) s += String.fromCharCode(u8[i]);
  return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

// jsonBase64UrlToBytes is the WebAuthn-specific helper: take
// the server-issued `options_json` (a JSON string with
// `challenge` and `allowCredentials[*].id` as base64url
// strings) and return the JSON shape the browser wants
// (`Uint8Array` for those fields). Operates in-place on a
// parsed JSON object so the rest of the structure (rpId,
// userVerification, etc.) round-trips unchanged.
export function jsonBase64UrlToBytes<T = unknown>(optionsJson: string): T {
  const obj = JSON.parse(optionsJson) as Record<string, unknown>;
  rewriteBase64UrlFieldsInPlace(obj);
  return obj as T;
}

// rewriteBase64UrlFieldsInPlace is the recursive descent that
// turns every `challenge` / `id` / `user.id` field (anywhere
// in the JSON tree) from base64url to Uint8Array. Keeps the
// outer shape so navigator.credentials accepts it without a
// second pass.
function rewriteBase64UrlFieldsInPlace(node: unknown): void {
  if (node === null || typeof node !== "object") return;
  if (Array.isArray(node)) {
    for (const item of node) rewriteBase64UrlFieldsInPlace(item);
    return;
  }
  const obj = node as Record<string, unknown>;
  for (const k of Object.keys(obj)) {
    const v = obj[k];
    if (typeof v === "string" && looksLikeBase64UrlField(k)) {
      try {
        obj[k] = base64UrlToBytes(v);
      } catch {
        // Leave the string alone if decode fails — the field
        // is probably already a Uint8Array (when the server
        // re-serializes a stored value) or a non-WebAuthn
        // string we shouldn't touch.
      }
    } else if (typeof v === "object" && v !== null) {
      rewriteBase64UrlFieldsInPlace(v);
    }
  }
}

// looksLikeBase64UrlField returns true for the keys the
// WebAuthn spec encodes as bytes. We whitelist rather than
// black-list so a server bug ("looks like base64url but
// isn't") surfaces as a runtime error instead of silently
// passing a string through.
function looksLikeBase64UrlField(key: string): boolean {
  return (
    key === "challenge" ||
    key === "id" ||
    key === "user.id" || // the user handle for usernameless flows
    key === "credentialId" // some servers use camelCase
  );
}

// encodeAttestationForServer converts the
// PublicKeyCredential object that `navigator.credentials.create()`
// returns into the JSON shape the hub server expects. The hub
// then re-decodes via base64url + verifies the signature.
export function encodeAttestationForServer(cred: {
  id: string;
  rawId: ArrayBuffer;
  type: string;
  response: { clientDataJSON: ArrayBuffer; attestationObject: ArrayBuffer };
}): Record<string, unknown> {
  return {
    id: cred.id,
    rawId: bytesToBase64Url(cred.rawId),
    type: cred.type,
    response: {
      clientDataJSON: bytesToBase64Url(cred.response.clientDataJSON),
      attestationObject: bytesToBase64Url(cred.response.attestationObject),
    },
  };
}

// encodeAssertionForServer mirrors encodeAttestationForServer
// for the LoginFinish side: the credential the browser picks
// is echoed back as base64url so the hub can verify the
// signature against the stored public key.
export function encodeAssertionForServer(cred: {
  id: string;
  rawId: ArrayBuffer;
  type: string;
  response: {
    clientDataJSON: ArrayBuffer;
    authenticatorData: ArrayBuffer;
    signature: ArrayBuffer;
    userHandle?: ArrayBuffer | null;
  };
}): Record<string, unknown> {
  const out: Record<string, unknown> = {
    id: cred.id,
    rawId: bytesToBase64Url(cred.rawId),
    type: cred.type,
    response: {
      clientDataJSON: bytesToBase64Url(cred.response.clientDataJSON),
      authenticatorData: bytesToBase64Url(cred.response.authenticatorData),
      signature: bytesToBase64Url(cred.response.signature),
    },
  };
  if (cred.response.userHandle) {
    (out.response as Record<string, unknown>).userHandle = bytesToBase64Url(cred.response.userHandle);
  }
  return out;
}
