import { createRoot } from "react-dom/client";

import { VisualHarness } from "./VisualHarness";

const root = document.getElementById("root");

if (!root) {
  throw new Error("Remotr visual evidence root is missing");
}

createRoot(root).render(<VisualHarness />);
