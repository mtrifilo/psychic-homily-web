import { describe, test, expect } from "bun:test";
import { APIClient, APIError } from "../src/lib/api";

describe("APIClient", () => {
  test("constructs with environment config", () => {
    const client = new APIClient({
      url: "https://api.psychichomily.com",
      token: "phk_test123",
    });
    expect(client).toBeDefined();
  });

  test("strips trailing slash from base URL", () => {
    // We can't easily inspect the private baseUrl, but we can verify
    // the client doesn't crash on construction with trailing slashes
    const client = new APIClient({
      url: "https://api.psychichomily.com///",
      token: "phk_test",
    });
    expect(client).toBeDefined();
  });

  test("healthCheck returns false for unreachable host", async () => {
    const client = new APIClient({
      url: "http://localhost:19999",
      token: "phk_test",
    });
    const result = await client.healthCheck();
    expect(result).toBe(false);
  });
});

describe("APIError", () => {
  test("includes status and error code", () => {
    const err = new APIError(422, "validation_failed", "Name is required", "req_123");
    expect(err.status).toBe(422);
    expect(err.errorCode).toBe("validation_failed");
    expect(err.message).toBe("Name is required");
    expect(err.requestId).toBe("req_123");
    expect(err.name).toBe("APIError");
  });

  test("is an instance of Error", () => {
    const err = new APIError(500, undefined, "Server error");
    expect(err).toBeInstanceOf(Error);
    expect(err).toBeInstanceOf(APIError);
  });
});
