import { Fingerprint, KeyRound, Plus, Shield, Trash2, Users } from "lucide-react";
import { type FormEvent, useEffect, useState } from "react";

import type {
  OperatorCredentialStampRequest,
  OperatorCredentialStampResult,
  OperatorRolesRequest,
  RBACMutationResult,
  RBACOperatorView,
  RBACRoleCreateRequest,
  RBACRoleDeleteRequest,
  RBACRoleView,
  RBACRuleAddRequest,
  RBACRuleRemoveRequest,
  RBACRuleView,
} from "./rbacOperator";
import "./RBACOperatorPage.css";

interface RBACOperatorPageProps {
  addRule: (request: RBACRuleAddRequest) => Promise<RBACRuleView>;
  createRole: (request: RBACRoleCreateRequest) => Promise<RBACRoleView>;
  deleteRole: (request: RBACRoleDeleteRequest) => Promise<RBACMutationResult>;
  getRole: (name: string) => Promise<RBACRoleView>;
  listOperators: () => Promise<RBACOperatorView[]>;
  listRoles: () => Promise<RBACRoleView[]>;
  refreshActivity: () => Promise<unknown>;
  removeRule: (request: RBACRuleRemoveRequest) => Promise<RBACMutationResult>;
  setOperatorRoles: (request: OperatorRolesRequest) => Promise<RBACOperatorView>;
  stampCredential: (
    request: OperatorCredentialStampRequest,
  ) => Promise<OperatorCredentialStampResult>;
}

type Confirmation =
  | { kind: "delete-role"; role: RBACRoleView }
  | { kind: "remove-rule"; role: RBACRoleView; rule: RBACRuleView }
  | { kind: "set-roles"; operator: RBACOperatorView };

export function RBACOperatorPage({
  addRule,
  createRole,
  deleteRole,
  getRole,
  listOperators,
  listRoles,
  refreshActivity,
  removeRule,
  setOperatorRoles,
  stampCredential,
}: RBACOperatorPageProps) {
  const [roles, setRoles] = useState<RBACRoleView[]>([]);
  const [operators, setOperators] = useState<RBACOperatorView[]>([]);
  const [selected, setSelected] = useState<RBACRoleView>();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [roleName, setRoleName] = useState("");
  const [roleDescription, setRoleDescription] = useState("");
  const [ruleMethod, setRuleMethod] = useState("GET");
  const [rulePath, setRulePath] = useState("");
  const [confirmation, setConfirmation] = useState<Confirmation>();
  const [confirmationText, setConfirmationText] = useState("");
  const [assignedRoles, setAssignedRoles] = useState<string[]>([]);
  const [credentialLabel, setCredentialLabel] = useState("");
  const [credentialRoles, setCredentialRoles] = useState<string[]>([]);
  const [credentialConfirmation, setCredentialConfirmation] = useState("");
  const [credentialResult, setCredentialResult] =
    useState<OperatorCredentialStampResult>();

  useEffect(() => {
    let current = true;
    void Promise.all([listRoles(), listOperators()])
      .then(([nextRoles, nextOperators]) => {
        if (!current) return;
        setRoles(nextRoles);
        setOperators(nextOperators);
      })
      .catch((cause) => current && setError(actionMessage(cause)));
    return () => {
      current = false;
    };
  }, [listOperators, listRoles]);

  const mutate = async (action: () => Promise<void>) => {
    setPending(true);
    setError("");
    try {
      await action();
      await refreshActivity();
    } catch (cause) {
      setError(actionMessage(cause));
    } finally {
      setPending(false);
    }
  };

  const create = async (event: FormEvent) => {
    event.preventDefault();
    await mutate(async () => {
      const role = await createRole({ description: roleDescription, name: roleName });
      setRoles((current) => [...current.filter((item) => item.name !== role.name), role].toSorted((left, right) => left.name.localeCompare(right.name)));
      setRoleName("");
      setRoleDescription("");
    });
  };

  const inspect = async (name: string) => {
    setPending(true);
    setError("");
    try {
      setSelected(await getRole(name));
    } catch (cause) {
      setError(actionMessage(cause));
    } finally {
      setPending(false);
    }
  };

  const add = async (event: FormEvent) => {
    event.preventDefault();
    if (!selected) return;
    await mutate(async () => {
      const rule = await addRule({ method: ruleMethod, pathPattern: rulePath, roleName: selected.name });
      setSelected({ ...selected, rules: [...selected.rules, rule] });
      setRulePath("");
    });
  };

  const openConfirmation = (next: Confirmation) => {
    setConfirmationText("");
    if (next.kind === "set-roles") setAssignedRoles([...next.operator.roles]);
    setConfirmation(next);
  };

  const submitConfirmation = async (event: FormEvent) => {
    event.preventDefault();
    if (!confirmation) return;
    await mutate(async () => {
      if (confirmation.kind === "delete-role") {
        await deleteRole({ confirmation: confirmationText, name: confirmation.role.name });
        setRoles((current) => current.filter((role) => role.name !== confirmation.role.name));
        if (selected?.name === confirmation.role.name) setSelected(undefined);
      } else if (confirmation.kind === "remove-rule") {
        await removeRule({ confirmation: confirmationText, roleName: confirmation.role.name, ruleId: confirmation.rule.id });
        setSelected((current) => current ? { ...current, rules: current.rules.filter((rule) => rule.id !== confirmation.rule.id) } : current);
      } else {
        const updated = await setOperatorRoles({ confirmation: confirmationText, operatorId: confirmation.operator.id, roles: [...assignedRoles].toSorted() });
        setOperators((current) => current.map((operator) => operator.id === updated.id ? updated : operator));
      }
      setConfirmation(undefined);
    });
  };

  const issueCredential = async (event: FormEvent) => {
    event.preventDefault();
    await mutate(async () => {
      const result = await stampCredential({ confirmation: credentialConfirmation, label: credentialLabel, roles: [...credentialRoles].toSorted() });
      setCredentialResult(result);
      setCredentialConfirmation("");
    });
  };

  return (
    <section aria-label="RBAC and Operator administration" className="rbac-page">
      <header className="rbac-heading">
        <div><span className="page-kicker">Server authority</span><h2>Security administration</h2><p>Manage canonical roles, API rules, Operator assignments, and protected automation credentials.</p></div>
        <Shield aria-hidden="true" size={28} strokeWidth={1.6} />
      </header>
      {error ? <p className="rbac-error" role="alert">{error}</p> : null}

      <div className="rbac-columns">
        <section aria-labelledby="roles-heading" className="rbac-panel">
          <div className="rbac-panel-heading"><Shield aria-hidden="true" size={18} /><h3 id="roles-heading">Roles</h3></div>
          <form className="rbac-create" onSubmit={create}>
            <label><span>New role name</span><input aria-label="New role name" value={roleName} onChange={(event) => setRoleName(event.target.value)} /></label>
            <label><span>Role description</span><input aria-label="Role description" value={roleDescription} onChange={(event) => setRoleDescription(event.target.value)} /></label>
            <button disabled={pending || !roleName} type="submit"><Plus aria-hidden="true" size={14} />Create role</button>
          </form>
          <div className="rbac-list">
            {roles.map((role) => (
              <article key={role.name}>
                <div><strong data-mono>{role.name}</strong>{role.builtIn ? <span>Built in</span> : null}</div>
                <p>{role.description}</p>
                <div><button aria-label={`Inspect ${role.name}`} onClick={() => void inspect(role.name)} type="button">Inspect</button>{!role.builtIn ? <button aria-label={`Delete ${role.name}`} onClick={() => openConfirmation({ kind: "delete-role", role })} type="button"><Trash2 aria-hidden="true" size={13} />Delete</button> : null}</div>
              </article>
            ))}
          </div>
        </section>

        <section aria-labelledby="operators-heading" className="rbac-panel">
          <div className="rbac-panel-heading"><Users aria-hidden="true" size={18} /><h3 id="operators-heading">Operators</h3></div>
          <div className="rbac-list">
            {operators.map((operator) => (
              <article key={operator.id}>
                <strong data-mono>{operator.id}</strong>
                <span className="rbac-fingerprint"><Fingerprint aria-hidden="true" size={13} />{operator.certFingerprint}</span>
                <p>{operator.roles.join(", ")}</p>
                <button aria-label={`Manage roles for ${operator.id}`} onClick={() => openConfirmation({ kind: "set-roles", operator })} type="button">Manage roles</button>
              </article>
            ))}
          </div>
        </section>
      </div>

      {selected ? (
        <section aria-label={`Role ${selected.name}`} className="rbac-panel rbac-detail">
          <div className="rbac-panel-heading"><h3 data-mono>{selected.name}</h3><span>{selected.rules.length} rules</span></div>
          <table aria-label={`${selected.name} rules`}><thead><tr><th>Method</th><th>Path pattern</th><th>Rule ID</th><th>Action</th></tr></thead><tbody>{selected.rules.map((rule) => <tr key={rule.id || `${rule.method}-${rule.pathPattern}`}><td data-mono>{rule.method}</td><td data-mono>{rule.pathPattern}</td><td data-mono>{rule.id || "Built in"}</td><td>{rule.id ? <button aria-label={`Remove rule ${rule.id}`} onClick={() => openConfirmation({ kind: "remove-rule", role: selected, rule })} type="button">Remove</button> : null}</td></tr>)}</tbody></table>
          {!selected.builtIn ? <form className="rbac-rule-form" onSubmit={add}><label><span>Rule method</span><select aria-label="Rule method" value={ruleMethod} onChange={(event) => setRuleMethod(event.target.value)}>{["GET", "POST", "PUT", "PATCH", "DELETE", "*"].map((method) => <option key={method}>{method}</option>)}</select></label><label><span>Rule path pattern</span><input aria-label="Rule path pattern" value={rulePath} onChange={(event) => setRulePath(event.target.value)} /></label><button disabled={pending || !rulePath} type="submit">Add rule</button></form> : null}
        </section>
      ) : null}

      <form className="rbac-panel rbac-credential" onSubmit={issueCredential}>
        <div className="rbac-panel-heading"><KeyRound aria-hidden="true" size={18} /><h3>Issue protected Operator credential</h3></div>
        <label><span>Credential label</span><input aria-label="Credential label" value={credentialLabel} onChange={(event) => setCredentialLabel(event.target.value)} /></label>
        <fieldset><legend>Canonical roles</legend>{roles.map((role) => <label key={role.name}><input aria-label={`Credential role ${role.name}`} checked={credentialRoles.includes(role.name)} onChange={() => setCredentialRoles(toggleValue(credentialRoles, role.name))} type="checkbox" />{role.name}</label>)}</fieldset>
        <label><span>Credential confirmation</span><input aria-label="Credential confirmation" value={credentialConfirmation} onChange={(event) => setCredentialConfirmation(event.target.value)} /></label>
        <button disabled={pending || !credentialLabel || credentialRoles.length === 0} type="submit">Choose directory and issue credential</button>
        {credentialResult ? <p role="status">Credential for <strong data-mono>{credentialResult.operatorId}</strong> saved in <strong>{credentialResult.directoryName}</strong>.</p> : null}
      </form>

      {confirmation ? <form aria-label={confirmationTitle(confirmation)} className="rbac-dialog" onSubmit={submitConfirmation} role="dialog"><span className="page-kicker">Exact confirmation</span><h3>{confirmationTitle(confirmation)}</h3>{confirmation.kind === "set-roles" ? <fieldset><legend>Roles</legend>{roles.map((role) => <label key={role.name}><input checked={assignedRoles.includes(role.name)} onChange={() => setAssignedRoles(toggleValue(assignedRoles, role.name))} type="checkbox" />{role.name}</label>)}</fieldset> : null}<label><span>{confirmationLabel(confirmation)}</span><input aria-label={confirmationLabel(confirmation)} value={confirmationText} onChange={(event) => setConfirmationText(event.target.value)} /></label><div><button onClick={() => setConfirmation(undefined)} type="button">Cancel</button><button disabled={pending} type="submit">{confirmationButton(confirmation)}</button></div></form> : null}
    </section>
  );
}

function toggleValue(values: string[], value: string): string[] {
  return values.includes(value) ? values.filter((item) => item !== value) : [...values, value];
}

function confirmationTitle(value: Confirmation): string {
  if (value.kind === "delete-role") return "Delete RBAC role";
  if (value.kind === "remove-rule") return "Remove RBAC rule";
  return "Set Operator roles";
}

function confirmationLabel(value: Confirmation): string {
  if (value.kind === "delete-role") return "Role deletion confirmation";
  if (value.kind === "remove-rule") return "Rule removal confirmation";
  return "Operator role confirmation";
}

function confirmationButton(value: Confirmation): string {
  if (value.kind === "delete-role") return "Delete role";
  if (value.kind === "remove-rule") return "Remove rule";
  return "Set roles";
}

function actionMessage(cause: unknown): string {
  return cause && typeof cause === "object" && "message" in cause && typeof cause.message === "string" ? cause.message : "The RBAC action could not be completed safely.";
}

