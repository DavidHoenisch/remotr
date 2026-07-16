// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import { act, cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  type ActionAcknowledgement,
  type ActionErrorEnvelope,
  useActionController,
} from "./useActionController";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

interface UpgradeInput {
  endpointId: string;
  version: string;
}

const acknowledgement: ActionAcknowledgement = {
  acceptedAt: "2032-03-04T05:06:07Z",
  action: "request_endpoint_upgrade",
  affectedEvidence: ["endpoint:endpoint-alpha", "activity"],
  requestId: "request-204",
  summary: "Upgrade to v2.2.0 accepted for endpoint-alpha.",
  target: "endpoint-alpha",
};

function ActionHarness({
  execute,
  refreshAffected,
}: {
  execute: (input: UpgradeInput) => Promise<ActionAcknowledgement>;
  refreshAffected: (result: ActionAcknowledgement) => Promise<void> | void;
}) {
  const [version, setVersion] = useState("v2.2.0");
  const action = useActionController({
    execute,
    refreshAffected,
    safeContext: (input: UpgradeInput) => ({
      endpointId: input.endpointId,
      version: input.version || "Not provided",
    }),
  });
  const input = { endpointId: "endpoint-alpha", version };

  return (
    <section aria-label="Endpoint upgrade action">
      <label>
        Version
        <input
          onChange={(event) => setVersion(event.target.value)}
          value={version}
        />
      </label>
      <button
        disabled={action.pending}
        onClick={() => void action.submit(input)}
        type="button"
      >
        {action.pending ? "Submitting upgrade request" : "Submit upgrade"}
      </button>
      <button
        onClick={() => {
          void action.submit(input);
          void action.submit(input);
        }}
        type="button"
      >
        Submit twice
      </button>
      {action.error ? (
        <section aria-label="Action failed" role="alert">
          <strong>{action.error.message}</strong>
          <p>{action.error.guidance}</p>
          {Object.entries(action.error.fieldErrors ?? {}).map(
            ([field, message]) => (
              <p key={field}>{`${field}: ${message}`}</p>
            ),
          )}
          <p>{`Endpoint: ${action.safeContext?.endpointId}`}</p>
          <p>{`Version: ${action.safeContext?.version}`}</p>
          {action.error.retryable ? (
            <button onClick={() => void action.submit(input)} type="button">
              Retry action
            </button>
          ) : null}
          <button onClick={action.reset} type="button">
            Cancel action
          </button>
        </section>
      ) : null}
      {action.result ? (
        <section aria-label="Action accepted" role="status">
          <strong>{action.result.summary}</strong>
          <p>{`Request: ${action.result.requestId}`}</p>
          <p>{`Accepted: ${action.result.acceptedAt}`}</p>
          <p>{`Refreshing: ${action.result.affectedEvidence.join(", ")}`}</p>
          <p>Server accepted the request. Awaiting fresh Endpoint evidence.</p>
        </section>
      ) : null}
    </section>
  );
}

describe("useActionController", () => {
  it("submits once, shows the exact acknowledgement, and refreshes affected evidence", async () => {
    const user = userEvent.setup();
    let resolveAction!: (result: ActionAcknowledgement) => void;
    const execute = vi.fn(
      () =>
        new Promise<ActionAcknowledgement>((resolve) => {
          resolveAction = resolve;
        }),
    );
    const refreshAffected = vi.fn();
    render(
      <ActionHarness
        execute={execute}
        refreshAffected={refreshAffected}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Submit twice" }));
    expect(execute).toHaveBeenCalledOnce();
    expect(execute).toHaveBeenCalledWith({
      endpointId: "endpoint-alpha",
      version: "v2.2.0",
    });
    expect(
      screen.getByRole("button", { name: "Submitting upgrade request" }),
    ).toBeDisabled();

    await act(async () => resolveAction(acknowledgement));
    const accepted = await screen.findByRole("status", {
      name: "Action accepted",
    });
    expect(
      within(accepted).getByText(
        "Upgrade to v2.2.0 accepted for endpoint-alpha.",
      ),
    ).toBeVisible();
    expect(within(accepted).getByText("Request: request-204")).toBeVisible();
    expect(
      within(accepted).getByText(
        "Refreshing: endpoint:endpoint-alpha, activity",
      ),
    ).toBeVisible();
    expect(refreshAffected).toHaveBeenCalledOnce();
    expect(refreshAffected).toHaveBeenCalledWith(acknowledgement);
    expect(accepted).not.toHaveTextContent(/converged|completed/i);
  });

  it("surfaces backend field validation without an Admin API mutation", async () => {
    const user = userEvent.setup();
    const adminMutation = vi.fn().mockResolvedValue(acknowledgement);
    const validation: ActionErrorEnvelope = {
      fieldErrors: {
        version: "Enter a semantic version such as v2.2.0.",
      },
      guidance: "Correct the highlighted field and submit again.",
      kind: "validation",
      message: "The upgrade request is invalid.",
      retryable: true,
    };
    const execute = vi.fn(async (input: UpgradeInput) => {
      if (!input.version) {
        throw validation;
      }
      return adminMutation(input);
    });
    render(
      <ActionHarness execute={execute} refreshAffected={() => undefined} />,
    );

    const version = screen.getByRole("textbox", { name: "Version" });
    await user.clear(version);
    await user.click(screen.getByRole("button", { name: "Submit upgrade" }));

    const failure = await screen.findByRole("alert", { name: "Action failed" });
    expect(
      within(failure).getByText(
        "version: Enter a semantic version such as v2.2.0.",
      ),
    ).toBeVisible();
    expect(within(failure).getByText("Endpoint: endpoint-alpha")).toBeVisible();
    expect(within(failure).getByText("Version: Not provided")).toBeVisible();
    expect(adminMutation).not.toHaveBeenCalled();

    await user.type(version, "v2.2.0");
    await user.click(within(failure).getByRole("button", { name: "Retry action" }));
    expect(adminMutation).toHaveBeenCalledOnce();
    expect(adminMutation).toHaveBeenCalledWith({
      endpointId: "endpoint-alpha",
      version: "v2.2.0",
    });
  });

  it("retains safe context and explicit recovery without exposing backend internals", async () => {
    const user = userEvent.setup();
    const unsafeCanary = "operator-token-canary-should-never-render";
    const failure = {
      debugContext: unsafeCanary,
      guidance: "Check authorization, then retry or cancel this request.",
      kind: "authorization",
      message: "The server rejected this Endpoint upgrade.",
      retryable: true,
    } satisfies ActionErrorEnvelope & { debugContext: string };
    const execute = vi
      .fn<(input: UpgradeInput) => Promise<ActionAcknowledgement>>()
      .mockRejectedValueOnce(failure)
      .mockResolvedValueOnce(acknowledgement);
    const refreshAffected = vi.fn();
    render(
      <ActionHarness
        execute={execute}
        refreshAffected={refreshAffected}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Submit upgrade" }));
    const alert = await screen.findByRole("alert", { name: "Action failed" });
    expect(within(alert).getByText("Endpoint: endpoint-alpha")).toBeVisible();
    expect(within(alert).getByText("Version: v2.2.0")).toBeVisible();
    expect(screen.queryByText(unsafeCanary)).not.toBeInTheDocument();
    expect(
      within(alert).getByRole("button", { name: "Retry action" }),
    ).toBeVisible();
    expect(
      within(alert).getByRole("button", { name: "Cancel action" }),
    ).toBeVisible();

    await user.click(within(alert).getByRole("button", { name: "Retry action" }));
    expect(execute).toHaveBeenCalledTimes(2);
    expect(await screen.findByRole("status", { name: "Action accepted" })).toBeVisible();
    expect(refreshAffected).toHaveBeenCalledWith(acknowledgement);
  });
});
