import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./App";
import { createWailsBridge } from "./bridge/desktopBridge";

const root = document.getElementById("root");

if (!root) {
  throw new Error("Remotr desktop root element is missing");
}

const bridge = createWailsBridge();

createRoot(root).render(
  <StrictMode>
    <App requestGitSync={bridge.requestGitSync} />
  </StrictMode>,
);
