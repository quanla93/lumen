import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import "./index.css";
import "uplot/dist/uPlot.min.css";

// Apply persisted theme (or system preference) BEFORE React mounts so there's
// no flash of the wrong palette on first paint.
const stored = localStorage.getItem("lumen.theme");
const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
if (stored === "dark" || (stored !== "light" && prefersDark)) {
  document.documentElement.classList.add("dark");
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
