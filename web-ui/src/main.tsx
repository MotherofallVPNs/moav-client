import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";

// Inject global styles (body background, scrollbar) inline so we don't need a
// separate CSS bundle.
const style = document.createElement("style");
style.textContent = `
  html, body, #root { height: 100%; margin: 0; padding: 0; }
  body {
    background: #0a0e17;
    color: #c9d1d9;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  }
  ::-webkit-scrollbar { width: 8px; height: 8px; }
  ::-webkit-scrollbar-track { background: #0a0e17; }
  ::-webkit-scrollbar-thumb { background: #1e2d3d; border-radius: 4px; }
  ::-webkit-scrollbar-thumb:hover { background: #2c4660; }
  a { color: #58a6ff; }
  code, pre { font-family: 'SF Mono', 'Cascadia Code', 'Fira Code', Consolas, monospace; }
  input, select, textarea {
    background: #1a2233; color: #c9d1d9; border: 1px solid #1e2d3d;
  }
  input:focus, select:focus, textarea:focus { outline: none; border-color: #58a6ff; }
  button { font-family: inherit; }
`;
document.head.appendChild(style);

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>
);
