import { describe, expect, it } from "vitest";
import { base64urlToBuffer, bufferToBase64url } from "@/lib/webauthn";

describe("webauthn base64url helpers", () => {
  it("round-trips arbitrary bytes", () => {
    const bytes = new Uint8Array([0, 1, 2, 250, 251, 252, 253, 254, 255]);
    const encoded = bufferToBase64url(bytes.buffer);
    expect(encoded).not.toContain("=");
    expect(encoded).not.toContain("+");
    expect(encoded).not.toContain("/");
    const decoded = new Uint8Array(base64urlToBuffer(encoded));
    expect(Array.from(decoded)).toEqual(Array.from(bytes));
  });

  it("decodes known base64url values", () => {
    // "hello" -> aGVsbG8
    expect(new TextDecoder().decode(base64urlToBuffer("aGVsbG8"))).toBe("hello");
    // url-safe alphabet: 0xFB 0xFF -> -_8
    expect(Array.from(new Uint8Array(base64urlToBuffer("-_8")))).toEqual([251, 255]);
  });
});
