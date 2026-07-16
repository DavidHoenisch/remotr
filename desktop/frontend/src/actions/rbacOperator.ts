export interface RBACRuleView {
  id: string;
  method: string;
  pathPattern: string;
  roleName: string;
}

export interface RBACRoleView {
  builtIn: boolean;
  description: string;
  name: string;
  rules: RBACRuleView[];
}

export interface RBACOperatorView {
  certFingerprint: string;
  createdAt: string;
  id: string;
  roles: string[];
}

export interface RBACRoleCreateRequest {
  description: string;
  name: string;
}

export interface RBACRoleDeleteRequest {
  confirmation: string;
  name: string;
}

export interface RBACRuleAddRequest {
  method: string;
  pathPattern: string;
  roleName: string;
}

export interface RBACRuleRemoveRequest {
  confirmation: string;
  roleName: string;
  ruleId: string;
}

export interface OperatorRolesRequest {
  confirmation: string;
  operatorId: string;
  roles: string[];
}

export interface OperatorCredentialStampRequest {
  confirmation: string;
  label: string;
  roles: string[];
}

export interface RBACMutationResult {
  name: string;
  ruleId: string;
  status: "deleted" | "removed";
}

export interface OperatorCredentialStampResult {
  directoryName: string;
  label: string;
  operatorId: string;
  roles: string[];
  status: "saved";
}

