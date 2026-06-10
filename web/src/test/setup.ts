// Vitest setup. Loaded once per test file (configured in
// vitest.config.ts). Adds the @testing-library/jest-dom matchers
// (toBeInTheDocument, toHaveTextContent, etc.) and wires the RTL
// cleanup so test cases don't leak DOM nodes between runs.

import "@testing-library/jest-dom/vitest";
import { afterEach } from "vitest";
import { cleanup } from "@testing-library/react";

afterEach(() => {
  cleanup();
});
