import { test } from "node:test";
import assert from "node:assert/strict";
import { formatCents } from "../src/money.js";

test("formats cents", () => {
  assert.equal(formatCents(1999), "19.99");
  assert.equal(formatCents(1905), "19.05");
  assert.equal(formatCents(7), "0.07");
  assert.equal(formatCents(-250), "-2.50");
});
