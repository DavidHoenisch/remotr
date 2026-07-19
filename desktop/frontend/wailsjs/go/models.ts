export namespace main {
	
	export class AIIntegrationView {
	    agent: string;
	    displayName: string;
	    scope: string;
	    installed: boolean;
	    bundleVersion: string;
	    source: string;
	    sourceVersion: string;
	    runtimeAvailable: boolean;
	    runtimeStatus: string;
	    guidance: string;
	
	    static createFrom(source: any = {}) {
	        return new AIIntegrationView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agent = source["agent"];
	        this.displayName = source["displayName"];
	        this.scope = source["scope"];
	        this.installed = source["installed"];
	        this.bundleVersion = source["bundleVersion"];
	        this.source = source["source"];
	        this.sourceVersion = source["sourceVersion"];
	        this.runtimeAvailable = source["runtimeAvailable"];
	        this.runtimeStatus = source["runtimeStatus"];
	        this.guidance = source["guidance"];
	    }
	}
	export class AIIntegrationActionResult {
	    integration: AIIntegrationView;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new AIIntegrationActionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.integration = this.convertValues(source["integration"], AIIntegrationView);
	        this.status = source["status"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AIIntegrationInstallRequest {
	    agent: string;
	    scope: string;
	    projectRootId: string;
	    replace: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AIIntegrationInstallRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agent = source["agent"];
	        this.scope = source["scope"];
	        this.projectRootId = source["projectRootId"];
	        this.replace = source["replace"];
	    }
	}
	export class AIIntegrationListRequest {
	    scope: string;
	    projectRootId: string;
	
	    static createFrom(source: any = {}) {
	        return new AIIntegrationListRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scope = source["scope"];
	        this.projectRootId = source["projectRootId"];
	    }
	}
	export class AIIntegrationUpgradeRequest {
	    agent: string;
	    scope: string;
	    projectRootId: string;
	    version: string;
	    replace: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AIIntegrationUpgradeRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agent = source["agent"];
	        this.scope = source["scope"];
	        this.projectRootId = source["projectRootId"];
	        this.version = source["version"];
	        this.replace = source["replace"];
	    }
	}
	
	export class AIProjectRootView {
	    id: string;
	    directoryName: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new AIProjectRootView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.directoryName = source["directoryName"];
	        this.status = source["status"];
	    }
	}
	export class ActivityDetail {
	    key: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new ActivityDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.value = source["value"];
	    }
	}
	export class ActivityEvent {
	    eventId: string;
	    occurredAt: string;
	    actor: string;
	    action: string;
	    resourceType: string;
	    resourceId: string;
	    status: string;
	    requestId: string;
	    details: ActivityDetail[];
	
	    static createFrom(source: any = {}) {
	        return new ActivityEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.eventId = source["eventId"];
	        this.occurredAt = source["occurredAt"];
	        this.actor = source["actor"];
	        this.action = source["action"];
	        this.resourceType = source["resourceType"];
	        this.resourceId = source["resourceId"];
	        this.status = source["status"];
	        this.requestId = source["requestId"];
	        this.details = this.convertValues(source["details"], ActivityDetail);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ActivityPageRequest {
	    since: string;
	    until: string;
	    action: string;
	    actorType: string;
	    cursor: string;
	    seenEventIds: string[];
	
	    static createFrom(source: any = {}) {
	        return new ActivityPageRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.since = source["since"];
	        this.until = source["until"];
	        this.action = source["action"];
	        this.actorType = source["actorType"];
	        this.cursor = source["cursor"];
	        this.seenEventIds = source["seenEventIds"];
	    }
	}
	export class ClassifiedError {
	    kind: string;
	    message: string;
	    guidance: string;
	
	    static createFrom(source: any = {}) {
	        return new ClassifiedError(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.message = source["message"];
	        this.guidance = source["guidance"];
	    }
	}
	export class SnapshotTimestamps {
	    loadedAt: string;
	    observedAt?: string;
	    failedAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new SnapshotTimestamps(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.loadedAt = source["loadedAt"];
	        this.observedAt = source["observedAt"];
	        this.failedAt = source["failedAt"];
	    }
	}
	export class SectionResult {
	    state: string;
	    snapshot: SnapshotTimestamps;
	    error?: ClassifiedError;
	
	    static createFrom(source: any = {}) {
	        return new SectionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.snapshot = this.convertValues(source["snapshot"], SnapshotTimestamps);
	        this.error = this.convertValues(source["error"], ClassifiedError);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ActivityPageView {
	    events: ActivityEvent[];
	    nextCursor: string;
	    section: SectionResult;
	
	    static createFrom(source: any = {}) {
	        return new ActivityPageView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.events = this.convertValues(source["events"], ActivityEvent);
	        this.nextCursor = source["nextCursor"];
	        this.section = this.convertValues(source["section"], SectionResult);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AppPackageArchiveView {
	    name: string;
	    version: string;
	    mode: string;
	    sha256: string;
	    sizeBytes: number;
	    fileName: string;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new AppPackageArchiveView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.mode = source["mode"];
	        this.sha256 = source["sha256"];
	        this.sizeBytes = source["sizeBytes"];
	        this.fileName = source["fileName"];
	        this.source = source["source"];
	    }
	}
	export class AppPackageDeleteRequest {
	    name: string;
	    version: string;
	    deleteObject: boolean;
	    confirmation: string;
	
	    static createFrom(source: any = {}) {
	        return new AppPackageDeleteRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.deleteObject = source["deleteObject"];
	        this.confirmation = source["confirmation"];
	    }
	}
	export class AppPackageDeleteResult {
	    name: string;
	    version: string;
	    scope: string;
	
	    static createFrom(source: any = {}) {
	        return new AppPackageDeleteResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.scope = source["scope"];
	    }
	}
	export class AppPackagePublishRequest {
	    name: string;
	    version: string;
	    sha256: string;
	    confirmation: string;
	
	    static createFrom(source: any = {}) {
	        return new AppPackagePublishRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.sha256 = source["sha256"];
	        this.confirmation = source["confirmation"];
	    }
	}
	export class AppPackageView {
	    id: string;
	    name: string;
	    version: string;
	    objectKey: string;
	    sha256: string;
	    installMode: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new AppPackageView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.version = source["version"];
	        this.objectKey = source["objectKey"];
	        this.sha256 = source["sha256"];
	        this.installMode = source["installMode"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class ApplicationInfo {
	    name: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new ApplicationInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	    }
	}
	export class AssetInventoryRow {
	    endpointId: string;
	    fleet: string;
	    os: string;
	    cpu: string;
	    ram: string;
	    kernel: string;
	    primaryIp: string;
	    macAddress: string;
	    diskEncryption: string;
	    tpm: string;
	    agentVersion: string;
	    lastCheckIn: string;
	
	    static createFrom(source: any = {}) {
	        return new AssetInventoryRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.endpointId = source["endpointId"];
	        this.fleet = source["fleet"];
	        this.os = source["os"];
	        this.cpu = source["cpu"];
	        this.ram = source["ram"];
	        this.kernel = source["kernel"];
	        this.primaryIp = source["primaryIp"];
	        this.macAddress = source["macAddress"];
	        this.diskEncryption = source["diskEncryption"];
	        this.tpm = source["tpm"];
	        this.agentVersion = source["agentVersion"];
	        this.lastCheckIn = source["lastCheckIn"];
	    }
	}
	export class AssetInventoryView {
	    rows: AssetInventoryRow[];
	    omittedEndpointIds: string[];
	    section: SectionResult;
	
	    static createFrom(source: any = {}) {
	        return new AssetInventoryView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rows = this.convertValues(source["rows"], AssetInventoryRow);
	        this.omittedEndpointIds = source["omittedEndpointIds"];
	        this.section = this.convertValues(source["section"], SectionResult);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AuditExportInfoView {
	    exportPath: string;
	    pathKey: string;
	
	    static createFrom(source: any = {}) {
	        return new AuditExportInfoView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exportPath = source["exportPath"];
	        this.pathKey = source["pathKey"];
	    }
	}
	export class BaselineAdoptionPreview {
	    planId: string;
	    fleet: string;
	    releaseRef: string;
	    artifactDigest: string;
	    targetCount: number;
	    resourceCount: number;
	    resourceAddresses: string[];
	
	    static createFrom(source: any = {}) {
	        return new BaselineAdoptionPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.planId = source["planId"];
	        this.fleet = source["fleet"];
	        this.releaseRef = source["releaseRef"];
	        this.artifactDigest = source["artifactDigest"];
	        this.targetCount = source["targetCount"];
	        this.resourceCount = source["resourceCount"];
	        this.resourceAddresses = source["resourceAddresses"];
	    }
	}
	export class BaselineAdoptionRequest {
	    planId: string;
	    fleet: string;
	    confirmation: string;
	
	    static createFrom(source: any = {}) {
	        return new BaselineAdoptionRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.planId = source["planId"];
	        this.fleet = source["fleet"];
	        this.confirmation = source["confirmation"];
	    }
	}
	export class BaselineAuthorizationView {
	    id: string;
	    changeRequestId: string;
	    fleet: string;
	    resourceAddress: string;
	    desiredHash: string;
	    risk: string;
	    provider: string;
	    authorizedBy: string;
	    authorizedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new BaselineAuthorizationView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.changeRequestId = source["changeRequestId"];
	        this.fleet = source["fleet"];
	        this.resourceAddress = source["resourceAddress"];
	        this.desiredHash = source["desiredHash"];
	        this.risk = source["risk"];
	        this.provider = source["provider"];
	        this.authorizedBy = source["authorizedBy"];
	        this.authorizedAt = source["authorizedAt"];
	    }
	}
	export class ChangeExecutionWindowView {
	    weekdays: number[];
	    startMinuteUtc: number;
	    durationMinutes: number;
	
	    static createFrom(source: any = {}) {
	        return new ChangeExecutionWindowView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.weekdays = source["weekdays"];
	        this.startMinuteUtc = source["startMinuteUtc"];
	        this.durationMinutes = source["durationMinutes"];
	    }
	}
	export class RolloutAuthorizationView {
	    id: string;
	    changeRequestId: string;
	    fleet: string;
	    validFrom: string;
	    validUntil: string;
	    attemptLimit: number;
	    maxConcurrency: number;
	    executionWindows: ChangeExecutionWindowView[];
	    authorizedBy: string;
	    authorizedAt: string;
	    justification: string;
	
	    static createFrom(source: any = {}) {
	        return new RolloutAuthorizationView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.changeRequestId = source["changeRequestId"];
	        this.fleet = source["fleet"];
	        this.validFrom = source["validFrom"];
	        this.validUntil = source["validUntil"];
	        this.attemptLimit = source["attemptLimit"];
	        this.maxConcurrency = source["maxConcurrency"];
	        this.executionWindows = this.convertValues(source["executionWindows"], ChangeExecutionWindowView);
	        this.authorizedBy = source["authorizedBy"];
	        this.authorizedAt = source["authorizedAt"];
	        this.justification = source["justification"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ChangeHistoryEvidence {
	    occurredAt: string;
	    actorId: string;
	    action: string;
	    details: string;
	
	    static createFrom(source: any = {}) {
	        return new ChangeHistoryEvidence(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.occurredAt = source["occurredAt"];
	        this.actorId = source["actorId"];
	        this.action = source["action"];
	        this.details = source["details"];
	    }
	}
	export class ChangeOutcomeEvidence {
	    endpointId: string;
	    state: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new ChangeOutcomeEvidence(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.endpointId = source["endpointId"];
	        this.state = source["state"];
	        this.reason = source["reason"];
	    }
	}
	export class ChangeApprovalEvidence {
	    operatorId: string;
	    approvedAt: string;
	    justification: string;
	
	    static createFrom(source: any = {}) {
	        return new ChangeApprovalEvidence(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.operatorId = source["operatorId"];
	        this.approvedAt = source["approvedAt"];
	        this.justification = source["justification"];
	    }
	}
	export class ChangeTargetEvidence {
	    endpointId: string;
	    compatible: boolean;
	    preflightReady: boolean;
	    preflightReason: string;
	
	    static createFrom(source: any = {}) {
	        return new ChangeTargetEvidence(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.endpointId = source["endpointId"];
	        this.compatible = source["compatible"];
	        this.preflightReady = source["preflightReady"];
	        this.preflightReason = source["preflightReason"];
	    }
	}
	export class ChangeResourceEvidence {
	    address: string;
	    desiredHash: string;
	    risk: string;
	    provider: string;
	    authorizationGroup: string;
	    dependsOn: string[];
	    activationTargets: string[];
	    predictedEffects: string[];
	    rollbackClass: string;
	    baselineEligible: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ChangeResourceEvidence(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.address = source["address"];
	        this.desiredHash = source["desiredHash"];
	        this.risk = source["risk"];
	        this.provider = source["provider"];
	        this.authorizationGroup = source["authorizationGroup"];
	        this.dependsOn = source["dependsOn"];
	        this.activationTargets = source["activationTargets"];
	        this.predictedEffects = source["predictedEffects"];
	        this.rollbackClass = source["rollbackClass"];
	        this.baselineEligible = source["baselineEligible"];
	    }
	}
	export class ChangeRequestSummary {
	    changeRequestId: string;
	    fleet: string;
	    releaseRef: string;
	    risk: string;
	    lifecycle: string;
	    targetCount: number;
	    requiredApprovals: number;
	    approvalCount: number;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new ChangeRequestSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.changeRequestId = source["changeRequestId"];
	        this.fleet = source["fleet"];
	        this.releaseRef = source["releaseRef"];
	        this.risk = source["risk"];
	        this.lifecycle = source["lifecycle"];
	        this.targetCount = source["targetCount"];
	        this.requiredApprovals = source["requiredApprovals"];
	        this.approvalCount = source["approvalCount"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class ChangeRequestDetailView {
	    summary: ChangeRequestSummary;
	    readOnly: boolean;
	    artifactDigest: string;
	    authorizationGroup: string;
	    policyWarning: string;
	    resources: ChangeResourceEvidence[];
	    resourcesTruncated: boolean;
	    targets: ChangeTargetEvidence[];
	    targetsTruncated: boolean;
	    approvals: ChangeApprovalEvidence[];
	    approvalsTruncated: boolean;
	    outcomes: ChangeOutcomeEvidence[];
	    outcomesTruncated: boolean;
	    history: ChangeHistoryEvidence[];
	    historyTruncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ChangeRequestDetailView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.summary = this.convertValues(source["summary"], ChangeRequestSummary);
	        this.readOnly = source["readOnly"];
	        this.artifactDigest = source["artifactDigest"];
	        this.authorizationGroup = source["authorizationGroup"];
	        this.policyWarning = source["policyWarning"];
	        this.resources = this.convertValues(source["resources"], ChangeResourceEvidence);
	        this.resourcesTruncated = source["resourcesTruncated"];
	        this.targets = this.convertValues(source["targets"], ChangeTargetEvidence);
	        this.targetsTruncated = source["targetsTruncated"];
	        this.approvals = this.convertValues(source["approvals"], ChangeApprovalEvidence);
	        this.approvalsTruncated = source["approvalsTruncated"];
	        this.outcomes = this.convertValues(source["outcomes"], ChangeOutcomeEvidence);
	        this.outcomesTruncated = source["outcomesTruncated"];
	        this.history = this.convertValues(source["history"], ChangeHistoryEvidence);
	        this.historyTruncated = source["historyTruncated"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ChangeActionResult {
	    action: string;
	    changeRequest: ChangeRequestDetailView;
	    authorization?: RolloutAuthorizationView;
	    baseline?: BaselineAuthorizationView;
	    affectedEvidence: string[];
	
	    static createFrom(source: any = {}) {
	        return new ChangeActionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.action = source["action"];
	        this.changeRequest = this.convertValues(source["changeRequest"], ChangeRequestDetailView);
	        this.authorization = this.convertValues(source["authorization"], RolloutAuthorizationView);
	        this.baseline = this.convertValues(source["baseline"], BaselineAuthorizationView);
	        this.affectedEvidence = source["affectedEvidence"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ChangeExecutionWindowInput {
	    weekdays: number[];
	    startMinuteUtc: number;
	    durationMinutes: number;
	
	    static createFrom(source: any = {}) {
	        return new ChangeExecutionWindowInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.weekdays = source["weekdays"];
	        this.startMinuteUtc = source["startMinuteUtc"];
	        this.durationMinutes = source["durationMinutes"];
	    }
	}
	export class ChangeAuthorizationRequest {
	    changeRequestId: string;
	    confirmation: string;
	    justification: string;
	    validFrom: string;
	    validUntil: string;
	    attemptLimit: number;
	    maxConcurrency: number;
	    executionWindows: ChangeExecutionWindowInput[];
	
	    static createFrom(source: any = {}) {
	        return new ChangeAuthorizationRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.changeRequestId = source["changeRequestId"];
	        this.confirmation = source["confirmation"];
	        this.justification = source["justification"];
	        this.validFrom = source["validFrom"];
	        this.validUntil = source["validUntil"];
	        this.attemptLimit = source["attemptLimit"];
	        this.maxConcurrency = source["maxConcurrency"];
	        this.executionWindows = this.convertValues(source["executionWindows"], ChangeExecutionWindowInput);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ChangeBaselinePromotionRequest {
	    changeRequestId: string;
	    resourceAddress: string;
	    confirmation: string;
	    acknowledgeExceptions: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ChangeBaselinePromotionRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.changeRequestId = source["changeRequestId"];
	        this.resourceAddress = source["resourceAddress"];
	        this.confirmation = source["confirmation"];
	        this.acknowledgeExceptions = source["acknowledgeExceptions"];
	    }
	}
	
	
	
	export class ChangeLifecycleRequest {
	    changeRequestId: string;
	    confirmation: string;
	    action: string;
	
	    static createFrom(source: any = {}) {
	        return new ChangeLifecycleRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.changeRequestId = source["changeRequestId"];
	        this.confirmation = source["confirmation"];
	        this.action = source["action"];
	    }
	}
	
	
	
	
	
	
	export class ConfigFleetDiscoverRequest {
	    workingTreeId: string;
	    fleet: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigFleetDiscoverRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workingTreeId = source["workingTreeId"];
	        this.fleet = source["fleet"];
	    }
	}
	export class ConfigValidationDiagnosticView {
	    path: string;
	    code: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigValidationDiagnosticView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.code = source["code"];
	        this.message = source["message"];
	    }
	}
	export class ConfigFleetDiscoveryView {
	    workingTreeId: string;
	    fleet: string;
	    manifest: string;
	    modules: string[];
	    applications: string[];
	    crons: string[];
	    resourceKinds: string[];
	    capabilityRequirements: string[];
	    diagnostics: ConfigValidationDiagnosticView[];
	
	    static createFrom(source: any = {}) {
	        return new ConfigFleetDiscoveryView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workingTreeId = source["workingTreeId"];
	        this.fleet = source["fleet"];
	        this.manifest = source["manifest"];
	        this.modules = source["modules"];
	        this.applications = source["applications"];
	        this.crons = source["crons"];
	        this.resourceKinds = source["resourceKinds"];
	        this.capabilityRequirements = source["capabilityRequirements"];
	        this.diagnostics = this.convertValues(source["diagnostics"], ConfigValidationDiagnosticView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ConfigHubImportRequest {
	    workingTreeId: string;
	    entryId: string;
	    outPath: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigHubImportRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workingTreeId = source["workingTreeId"];
	        this.entryId = source["entryId"];
	        this.outPath = source["outPath"];
	    }
	}
	export class ConfigHubImportResult {
	    entryId: string;
	    outPath: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigHubImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entryId = source["entryId"];
	        this.outPath = source["outPath"];
	        this.status = source["status"];
	    }
	}
	export class ConfigHubSnippetView {
	    id: string;
	    title: string;
	    description: string;
	    category: string;
	    tags: string[];
	    distros: string[];
	    author: string;
	    featured: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ConfigHubSnippetView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.category = source["category"];
	        this.tags = source["tags"];
	        this.distros = source["distros"];
	        this.author = source["author"];
	        this.featured = source["featured"];
	    }
	}
	export class ConfigRenderRequest {
	    workingTreeId: string;
	    scope: string;
	    targetId: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigRenderRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workingTreeId = source["workingTreeId"];
	        this.scope = source["scope"];
	        this.targetId = source["targetId"];
	    }
	}
	export class ConfigRenderSaveRequest {
	    workingTreeId: string;
	    targetType: string;
	    targetId: string;
	    artifactType: string;
	    digest: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigRenderSaveRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workingTreeId = source["workingTreeId"];
	        this.targetType = source["targetType"];
	        this.targetId = source["targetId"];
	        this.artifactType = source["artifactType"];
	        this.digest = source["digest"];
	    }
	}
	export class ConfigRenderSaveResult {
	    fileName: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigRenderSaveResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fileName = source["fileName"];
	        this.status = source["status"];
	    }
	}
	export class ConfigRenderedArtifactView {
	    targetType: string;
	    targetId: string;
	    artifactType: string;
	    content: string;
	    digest: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigRenderedArtifactView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.targetType = source["targetType"];
	        this.targetId = source["targetId"];
	        this.artifactType = source["artifactType"];
	        this.content = source["content"];
	        this.digest = source["digest"];
	    }
	}
	export class ConfigRenderView {
	    workingTreeId: string;
	    artifacts: ConfigRenderedArtifactView[];
	
	    static createFrom(source: any = {}) {
	        return new ConfigRenderView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workingTreeId = source["workingTreeId"];
	        this.artifacts = this.convertValues(source["artifacts"], ConfigRenderedArtifactView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ConfigRepositoryInitRequest {
	    fleet: string;
	    remediationPolicy: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigRepositoryInitRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fleet = source["fleet"];
	        this.remediationPolicy = source["remediationPolicy"];
	    }
	}
	export class ConfigWorkingTreeView {
	    id: string;
	    directoryName: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigWorkingTreeView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.directoryName = source["directoryName"];
	        this.status = source["status"];
	    }
	}
	export class ConfigRepositoryInitResult {
	    workingTree: ConfigWorkingTreeView;
	    fleet: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigRepositoryInitResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workingTree = this.convertValues(source["workingTree"], ConfigWorkingTreeView);
	        this.fleet = source["fleet"];
	        this.status = source["status"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ConfigValidationFinding {
	    path: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigValidationFinding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.message = source["message"];
	    }
	}
	export class ConfigValidationView {
	    workingTreeId: string;
	    valid: boolean;
	    ok: string[];
	    issues: ConfigValidationFinding[];
	    diagnostics: ConfigValidationDiagnosticView[];
	
	    static createFrom(source: any = {}) {
	        return new ConfigValidationView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workingTreeId = source["workingTreeId"];
	        this.valid = source["valid"];
	        this.ok = source["ok"];
	        this.issues = this.convertValues(source["issues"], ConfigValidationFinding);
	        this.diagnostics = this.convertValues(source["diagnostics"], ConfigValidationDiagnosticView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ConnectionProfile {
	    name: string;
	    serverUrl: string;
	    stateDir: string;
	    caPath: string;
	    defaultFleet: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.serverUrl = source["serverUrl"];
	        this.stateDir = source["stateDir"];
	        this.caPath = source["caPath"];
	        this.defaultFleet = source["defaultFleet"];
	    }
	}
	export class ConnectionView {
	    profileName: string;
	    serverUrl: string;
	    operatorId: string;
	    roles: string[];
	
	    static createFrom(source: any = {}) {
	        return new ConnectionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileName = source["profileName"];
	        this.serverUrl = source["serverUrl"];
	        this.operatorId = source["operatorId"];
	        this.roles = source["roles"];
	    }
	}
	export class DeploymentTokenCreateRequest {
	    label: string;
	    fleet: string;
	    ttlSeconds: number;
	
	    static createFrom(source: any = {}) {
	        return new DeploymentTokenCreateRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.fleet = source["fleet"];
	        this.ttlSeconds = source["ttlSeconds"];
	    }
	}
	export class DeploymentTokenView {
	    id: string;
	    label: string;
	    fleet: string;
	    status: string;
	    createdAt: string;
	    expiresAt: string;
	    revokedAt?: string;
	    lastUsedAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new DeploymentTokenView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.fleet = source["fleet"];
	        this.status = source["status"];
	        this.createdAt = source["createdAt"];
	        this.expiresAt = source["expiresAt"];
	        this.revokedAt = source["revokedAt"];
	        this.lastUsedAt = source["lastUsedAt"];
	    }
	}
	export class DeploymentTokenCreateResult {
	    token: string;
	    metadata: DeploymentTokenView;
	
	    static createFrom(source: any = {}) {
	        return new DeploymentTokenCreateResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.token = source["token"];
	        this.metadata = this.convertValues(source["metadata"], DeploymentTokenView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DeploymentTokenRevokeRequest {
	    label: string;
	    confirmation: string;
	
	    static createFrom(source: any = {}) {
	        return new DeploymentTokenRevokeRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.confirmation = source["confirmation"];
	    }
	}
	export class DeploymentTokenSaveResult {
	    status: string;
	    path?: string;
	    sizeBytes?: number;
	
	    static createFrom(source: any = {}) {
	        return new DeploymentTokenSaveResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.path = source["path"];
	        this.sizeBytes = source["sizeBytes"];
	    }
	}
	
	export class DesktopDoctorCheck {
	    name: string;
	    status: string;
	    detail: string;
	    guidance: string;
	
	    static createFrom(source: any = {}) {
	        return new DesktopDoctorCheck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.status = source["status"];
	        this.detail = source["detail"];
	        this.guidance = source["guidance"];
	    }
	}
	export class DesktopDoctorReport {
	    profileName: string;
	    healthy: boolean;
	    operatorId: string;
	    roles: string[];
	    checks: DesktopDoctorCheck[];
	
	    static createFrom(source: any = {}) {
	        return new DesktopDoctorReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileName = source["profileName"];
	        this.healthy = source["healthy"];
	        this.operatorId = source["operatorId"];
	        this.roles = source["roles"];
	        this.checks = this.convertValues(source["checks"], DesktopDoctorCheck);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DesktopUpdateStatus {
	    currentVersion: string;
	    latestVersion: string;
	    updateAvailable: boolean;
	    installSupported: boolean;
	    platform: string;
	    guidance: string;
	
	    static createFrom(source: any = {}) {
	        return new DesktopUpdateStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.currentVersion = source["currentVersion"];
	        this.latestVersion = source["latestVersion"];
	        this.updateAvailable = source["updateAvailable"];
	        this.installSupported = source["installSupported"];
	        this.platform = source["platform"];
	        this.guidance = source["guidance"];
	    }
	}
	export class DiagnosticBundleSaveResult {
	    status: string;
	    path?: string;
	    sizeBytes?: number;
	
	    static createFrom(source: any = {}) {
	        return new DiagnosticBundleSaveResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.path = source["path"];
	        this.sizeBytes = source["sizeBytes"];
	    }
	}
	export class DiagnosticCapabilities {
	    collectors: string[];
	    maxTimeSpanSeconds: number;
	
	    static createFrom(source: any = {}) {
	        return new DiagnosticCapabilities(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.collectors = source["collectors"];
	        this.maxTimeSpanSeconds = source["maxTimeSpanSeconds"];
	    }
	}
	export class DiagnosticCollectionRequest {
	    endpointId: string;
	    collectors: string[];
	    since: string;
	    until: string;
	
	    static createFrom(source: any = {}) {
	        return new DiagnosticCollectionRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.endpointId = source["endpointId"];
	        this.collectors = source["collectors"];
	        this.since = source["since"];
	        this.until = source["until"];
	    }
	}
	export class DiagnosticCollectionResult {
	    requestId: string;
	    endpointId: string;
	    status: string;
	    collectors: string[];
	    since: string;
	    until: string;
	    createdAt?: string;
	    expiresAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new DiagnosticCollectionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requestId = source["requestId"];
	        this.endpointId = source["endpointId"];
	        this.status = source["status"];
	        this.collectors = source["collectors"];
	        this.since = source["since"];
	        this.until = source["until"];
	        this.createdAt = source["createdAt"];
	        this.expiresAt = source["expiresAt"];
	    }
	}
	export class DiagnosticLifecycleView {
	    requestId: string;
	    endpointId: string;
	    requestedBy: string;
	    status: string;
	    collectors: string[];
	    since: string;
	    until: string;
	    sha256: string;
	    sizeBytes: number;
	    errorMessage: string;
	    createdAt: string;
	    dispatchedAt: string;
	    completedAt: string;
	    expiresAt: string;
	
	    static createFrom(source: any = {}) {
	        return new DiagnosticLifecycleView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requestId = source["requestId"];
	        this.endpointId = source["endpointId"];
	        this.requestedBy = source["requestedBy"];
	        this.status = source["status"];
	        this.collectors = source["collectors"];
	        this.since = source["since"];
	        this.until = source["until"];
	        this.sha256 = source["sha256"];
	        this.sizeBytes = source["sizeBytes"];
	        this.errorMessage = source["errorMessage"];
	        this.createdAt = source["createdAt"];
	        this.dispatchedAt = source["dispatchedAt"];
	        this.completedAt = source["completedAt"];
	        this.expiresAt = source["expiresAt"];
	    }
	}
	export class EndpointDetailSections {
	    overview: SectionResult;
	    state: SectionResult;
	    schedules: SectionResult;
	    firewall: SectionResult;
	    system: SectionResult;
	
	    static createFrom(source: any = {}) {
	        return new EndpointDetailSections(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.overview = this.convertValues(source["overview"], SectionResult);
	        this.state = this.convertValues(source["state"], SectionResult);
	        this.schedules = this.convertValues(source["schedules"], SectionResult);
	        this.firewall = this.convertValues(source["firewall"], SectionResult);
	        this.system = this.convertValues(source["system"], SectionResult);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SystemEvidence {
	    hostname: string;
	    os: string;
	    kernel: string;
	    cpu: string;
	    cpuCores: string;
	    memory: string;
	    digest: string;
	    reportedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new SystemEvidence(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hostname = source["hostname"];
	        this.os = source["os"];
	        this.kernel = source["kernel"];
	        this.cpu = source["cpu"];
	        this.cpuCores = source["cpuCores"];
	        this.memory = source["memory"];
	        this.digest = source["digest"];
	        this.reportedAt = source["reportedAt"];
	    }
	}
	export class FirewallEvidence {
	    timestamp: string;
	    ruleName: string;
	    action: string;
	    protocol: string;
	    ports: number[];
	    sources: string[];
	    backend: string;
	    wouldHave: string;
	    enforced: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FirewallEvidence(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timestamp = source["timestamp"];
	        this.ruleName = source["ruleName"];
	        this.action = source["action"];
	        this.protocol = source["protocol"];
	        this.ports = source["ports"];
	        this.sources = source["sources"];
	        this.backend = source["backend"];
	        this.wouldHave = source["wouldHave"];
	        this.enforced = source["enforced"];
	    }
	}
	export class ScheduleEvidence {
	    name: string;
	    schedule: string;
	    applicable: boolean;
	    lastStatus: string;
	    lastMessage: string;
	    lastScheduledFor: string;
	    lastCompletedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new ScheduleEvidence(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.schedule = source["schedule"];
	        this.applicable = source["applicable"];
	        this.lastStatus = source["lastStatus"];
	        this.lastMessage = source["lastMessage"];
	        this.lastScheduledFor = source["lastScheduledFor"];
	        this.lastCompletedAt = source["lastCompletedAt"];
	    }
	}
	export class StateEvidenceSubresult {
	    target: string;
	    status: string;
	    reasonCode: string;
	    desiredSummary: string;
	    observedSummary: string;
	
	    static createFrom(source: any = {}) {
	        return new StateEvidenceSubresult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.target = source["target"];
	        this.status = source["status"];
	        this.reasonCode = source["reasonCode"];
	        this.desiredSummary = source["desiredSummary"];
	        this.observedSummary = source["observedSummary"];
	    }
	}
	export class StateEvidenceItem {
	    address: string;
	    name: string;
	    description: string;
	    provider: string;
	    status: string;
	    reasonCode: string;
	    desiredSummary: string;
	    observedSummary: string;
	    subresults: StateEvidenceSubresult[];
	    subresultsTruncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new StateEvidenceItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.address = source["address"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.provider = source["provider"];
	        this.status = source["status"];
	        this.reasonCode = source["reasonCode"];
	        this.desiredSummary = source["desiredSummary"];
	        this.observedSummary = source["observedSummary"];
	        this.subresults = this.convertValues(source["subresults"], StateEvidenceSubresult);
	        this.subresultsTruncated = source["subresultsTruncated"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class StateEvidence {
	    endpointId: string;
	    releaseRef: string;
	    digest: string;
	    status: string;
	    reportedAt: string;
	    items: StateEvidenceItem[];
	
	    static createFrom(source: any = {}) {
	        return new StateEvidence(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.endpointId = source["endpointId"];
	        this.releaseRef = source["releaseRef"];
	        this.digest = source["digest"];
	        this.status = source["status"];
	        this.reportedAt = source["reportedAt"];
	        this.items = this.convertValues(source["items"], StateEvidenceItem);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LabelView {
	    key: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new LabelView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.value = source["value"];
	    }
	}
	export class MissingRequirementView {
	    id: string;
	    revision?: string;
	
	    static createFrom(source: any = {}) {
	        return new MissingRequirementView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.revision = source["revision"];
	    }
	}
	export class EndpointRow {
	    endpointId: string;
	    fleet: string;
	    usernames: string[];
	    compliance: string;
	    freshness: string;
	    desiredAgentVersion: string;
	    reportedAgentVersion: string;
	    releaseRef: string;
	    targetReleaseRef?: string;
	    offeredReleaseRef?: string;
	    offeredDigest?: string;
	    offeredSchemaVersion?: number;
	    activeReleaseRef?: string;
	    activeDigest?: string;
	    activeSchemaVersion?: number;
	    capabilityDigest?: string;
	    capabilityReceivedAt?: string;
	    capabilityBlockedTargetRef?: string;
	    missingRequirements?: MissingRequirementView[];
	    unmanaged?: boolean;
	    labels: LabelView[];
	    evidenceAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new EndpointRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.endpointId = source["endpointId"];
	        this.fleet = source["fleet"];
	        this.usernames = source["usernames"];
	        this.compliance = source["compliance"];
	        this.freshness = source["freshness"];
	        this.desiredAgentVersion = source["desiredAgentVersion"];
	        this.reportedAgentVersion = source["reportedAgentVersion"];
	        this.releaseRef = source["releaseRef"];
	        this.targetReleaseRef = source["targetReleaseRef"];
	        this.offeredReleaseRef = source["offeredReleaseRef"];
	        this.offeredDigest = source["offeredDigest"];
	        this.offeredSchemaVersion = source["offeredSchemaVersion"];
	        this.activeReleaseRef = source["activeReleaseRef"];
	        this.activeDigest = source["activeDigest"];
	        this.activeSchemaVersion = source["activeSchemaVersion"];
	        this.capabilityDigest = source["capabilityDigest"];
	        this.capabilityReceivedAt = source["capabilityReceivedAt"];
	        this.capabilityBlockedTargetRef = source["capabilityBlockedTargetRef"];
	        this.missingRequirements = this.convertValues(source["missingRequirements"], MissingRequirementView);
	        this.unmanaged = source["unmanaged"];
	        this.labels = this.convertValues(source["labels"], LabelView);
	        this.evidenceAt = source["evidenceAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class EndpointDetailView {
	    header: EndpointRow;
	    sections: EndpointDetailSections;
	    state: StateEvidence;
	    stateTruncated: boolean;
	    schedules: ScheduleEvidence[];
	    schedulesTruncated: boolean;
	    firewall: FirewallEvidence[];
	    firewallTruncated: boolean;
	    system: SystemEvidence;
	
	    static createFrom(source: any = {}) {
	        return new EndpointDetailView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.header = this.convertValues(source["header"], EndpointRow);
	        this.sections = this.convertValues(source["sections"], EndpointDetailSections);
	        this.state = this.convertValues(source["state"], StateEvidence);
	        this.stateTruncated = source["stateTruncated"];
	        this.schedules = this.convertValues(source["schedules"], ScheduleEvidence);
	        this.schedulesTruncated = source["schedulesTruncated"];
	        this.firewall = this.convertValues(source["firewall"], FirewallEvidence);
	        this.firewallTruncated = source["firewallTruncated"];
	        this.system = this.convertValues(source["system"], SystemEvidence);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class EndpointLabelRemoveRequest {
	    endpointId: string;
	    key: string;
	
	    static createFrom(source: any = {}) {
	        return new EndpointLabelRemoveRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.endpointId = source["endpointId"];
	        this.key = source["key"];
	    }
	}
	export class EndpointLabelResultView {
	    effect: string;
	    endpointId: string;
	    key: string;
	    value: string;
	    labels: LabelView[];
	
	    static createFrom(source: any = {}) {
	        return new EndpointLabelResultView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.effect = source["effect"];
	        this.endpointId = source["endpointId"];
	        this.key = source["key"];
	        this.value = source["value"];
	        this.labels = this.convertValues(source["labels"], LabelView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class EndpointLabelSetRequest {
	    endpointId: string;
	    key: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new EndpointLabelSetRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.endpointId = source["endpointId"];
	        this.key = source["key"];
	        this.value = source["value"];
	    }
	}
	export class EndpointRemovalRequest {
	    endpointId: string;
	    confirmation: string;
	
	    static createFrom(source: any = {}) {
	        return new EndpointRemovalRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.endpointId = source["endpointId"];
	        this.confirmation = source["confirmation"];
	    }
	}
	export class EndpointRemovalResult {
	    status: string;
	    endpointId: string;
	    credentialStatus: string;
	    affectedEvidence: string[];
	
	    static createFrom(source: any = {}) {
	        return new EndpointRemovalResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.endpointId = source["endpointId"];
	        this.credentialStatus = source["credentialStatus"];
	        this.affectedEvidence = source["affectedEvidence"];
	    }
	}
	
	export class EndpointUpgradeRequest {
	    endpointId: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new EndpointUpgradeRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.endpointId = source["endpointId"];
	        this.version = source["version"];
	    }
	}
	export class EndpointUpgradeResult {
	    status: string;
	    endpointId: string;
	    version: string;
	    affectedEvidence: string[];
	
	    static createFrom(source: any = {}) {
	        return new EndpointUpgradeResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.endpointId = source["endpointId"];
	        this.version = source["version"];
	        this.affectedEvidence = source["affectedEvidence"];
	    }
	}
	export class EnrollmentTokenRequest {
	    fleet: string;
	    ttlSeconds: number;
	
	    static createFrom(source: any = {}) {
	        return new EnrollmentTokenRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fleet = source["fleet"];
	        this.ttlSeconds = source["ttlSeconds"];
	    }
	}
	export class EnrollmentTokenResult {
	    token: string;
	    fleet: string;
	    expiresAt: string;
	
	    static createFrom(source: any = {}) {
	        return new EnrollmentTokenResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.token = source["token"];
	        this.fleet = source["fleet"];
	        this.expiresAt = source["expiresAt"];
	    }
	}
	
	export class FirewallExportRequest {
	    endpointId: string;
	    format: string;
	
	    static createFrom(source: any = {}) {
	        return new FirewallExportRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.endpointId = source["endpointId"];
	        this.format = source["format"];
	    }
	}
	export class FirewallZoneEvidence {
	    name: string;
	    target: string;
	    services: string[];
	    ports: string[];
	    sources: string[];
	    richRules: string[];
	
	    static createFrom(source: any = {}) {
	        return new FirewallZoneEvidence(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.target = source["target"];
	        this.services = source["services"];
	        this.ports = source["ports"];
	        this.sources = source["sources"];
	        this.richRules = source["richRules"];
	    }
	}
	export class FirewallLiveEvidence {
	    backend: string;
	    defaultZone: string;
	    zones: FirewallZoneEvidence[];
	    ruleset: string;
	    rulesetTruncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FirewallLiveEvidence(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.backend = source["backend"];
	        this.defaultZone = source["defaultZone"];
	        this.zones = this.convertValues(source["zones"], FirewallZoneEvidence);
	        this.ruleset = source["ruleset"];
	        this.rulesetTruncated = source["rulesetTruncated"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FirewallReportSections {
	    audit: SectionResult;
	    live: SectionResult;
	
	    static createFrom(source: any = {}) {
	        return new FirewallReportSections(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.audit = this.convertValues(source["audit"], SectionResult);
	        this.live = this.convertValues(source["live"], SectionResult);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FirewallReportView {
	    endpointId: string;
	    audit: FirewallEvidence[];
	    live: FirewallLiveEvidence;
	    sections: FirewallReportSections;
	
	    static createFrom(source: any = {}) {
	        return new FirewallReportView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.endpointId = source["endpointId"];
	        this.audit = this.convertValues(source["audit"], FirewallEvidence);
	        this.live = this.convertValues(source["live"], FirewallLiveEvidence);
	        this.sections = this.convertValues(source["sections"], FirewallReportSections);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class FleetDetailSections {
	    members: SectionResult;
	    state: SectionResult;
	
	    static createFrom(source: any = {}) {
	        return new FleetDetailSections(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.members = this.convertValues(source["members"], SectionResult);
	        this.state = this.convertValues(source["state"], SectionResult);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class StatusCount {
	    status: string;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new StatusCount(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.count = source["count"];
	    }
	}
	export class FleetSummary {
	    fleet: string;
	    endpointCount: number;
	    compliance: StatusCount[];
	    freshness: StatusCount[];
	    agentVersions: StatusCount[];
	
	    static createFrom(source: any = {}) {
	        return new FleetSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fleet = source["fleet"];
	        this.endpointCount = source["endpointCount"];
	        this.compliance = this.convertValues(source["compliance"], StatusCount);
	        this.freshness = this.convertValues(source["freshness"], StatusCount);
	        this.agentVersions = this.convertValues(source["agentVersions"], StatusCount);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FleetDetailView {
	    fleet: string;
	    summary: FleetSummary;
	    members: EndpointRow[];
	    sections: FleetDetailSections;
	    empty: boolean;
	    emptyMessage: string;
	
	    static createFrom(source: any = {}) {
	        return new FleetDetailView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fleet = source["fleet"];
	        this.summary = this.convertValues(source["summary"], FleetSummary);
	        this.members = this.convertValues(source["members"], EndpointRow);
	        this.sections = this.convertValues(source["sections"], FleetDetailSections);
	        this.empty = source["empty"];
	        this.emptyMessage = source["emptyMessage"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FleetOperationalReportSections {
	    state: SectionResult;
	    schedules: SectionResult;
	
	    static createFrom(source: any = {}) {
	        return new FleetOperationalReportSections(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = this.convertValues(source["state"], SectionResult);
	        this.schedules = this.convertValues(source["schedules"], SectionResult);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FleetScheduleEvidence {
	    endpointId: string;
	    name: string;
	    schedule: string;
	    applicable: boolean;
	    lastStatus: string;
	    lastMessage: string;
	    lastScheduledFor: string;
	    lastCompletedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new FleetScheduleEvidence(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.endpointId = source["endpointId"];
	        this.name = source["name"];
	        this.schedule = source["schedule"];
	        this.applicable = source["applicable"];
	        this.lastStatus = source["lastStatus"];
	        this.lastMessage = source["lastMessage"];
	        this.lastScheduledFor = source["lastScheduledFor"];
	        this.lastCompletedAt = source["lastCompletedAt"];
	    }
	}
	export class FleetOperationalReportsView {
	    fleet: string;
	    states: StateEvidence[];
	    schedules: FleetScheduleEvidence[];
	    sections: FleetOperationalReportSections;
	
	    static createFrom(source: any = {}) {
	        return new FleetOperationalReportsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fleet = source["fleet"];
	        this.states = this.convertValues(source["states"], StateEvidence);
	        this.schedules = this.convertValues(source["schedules"], FleetScheduleEvidence);
	        this.sections = this.convertValues(source["sections"], FleetOperationalReportSections);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class FleetUpgradeRequest {
	    fleet: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new FleetUpgradeRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fleet = source["fleet"];
	        this.version = source["version"];
	    }
	}
	export class FleetUpgradeResult {
	    status: string;
	    fleet: string;
	    version: string;
	    acceptedEndpoints: number;
	
	    static createFrom(source: any = {}) {
	        return new FleetUpgradeResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.fleet = source["fleet"];
	        this.version = source["version"];
	        this.acceptedEndpoints = source["acceptedEndpoints"];
	    }
	}
	export class GitSyncResult {
	    status: string;
	    action: string;
	    target: string;
	    profileName: string;
	    summary: string;
	    acceptedAt: string;
	    affectedEvidence: string[];
	
	    static createFrom(source: any = {}) {
	        return new GitSyncResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.action = source["action"];
	        this.target = source["target"];
	        this.profileName = source["profileName"];
	        this.summary = source["summary"];
	        this.acceptedAt = source["acceptedAt"];
	        this.affectedEvidence = source["affectedEvidence"];
	    }
	}
	
	export class LocalPackageCreateRequest {
	    directoryName: string;
	    name: string;
	    version: string;
	    mode: string;
	
	    static createFrom(source: any = {}) {
	        return new LocalPackageCreateRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.directoryName = source["directoryName"];
	        this.name = source["name"];
	        this.version = source["version"];
	        this.mode = source["mode"];
	    }
	}
	export class LocalPackageView {
	    name: string;
	    version: string;
	    mode: string;
	    locationName: string;
	
	    static createFrom(source: any = {}) {
	        return new LocalPackageView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.mode = source["mode"];
	        this.locationName = source["locationName"];
	    }
	}
	export class OperatorCredentialStampRequest {
	    label: string;
	    roles: string[];
	    confirmation: string;
	
	    static createFrom(source: any = {}) {
	        return new OperatorCredentialStampRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.roles = source["roles"];
	        this.confirmation = source["confirmation"];
	    }
	}
	export class OperatorCredentialStampResult {
	    operatorId: string;
	    label: string;
	    roles: string[];
	    directoryName: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new OperatorCredentialStampResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.operatorId = source["operatorId"];
	        this.label = source["label"];
	        this.roles = source["roles"];
	        this.directoryName = source["directoryName"];
	        this.status = source["status"];
	    }
	}
	export class OperatorRolesRequest {
	    operatorId: string;
	    roles: string[];
	    confirmation: string;
	
	    static createFrom(source: any = {}) {
	        return new OperatorRolesRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.operatorId = source["operatorId"];
	        this.roles = source["roles"];
	        this.confirmation = source["confirmation"];
	    }
	}
	export class OperatorView {
	    operatorId: string;
	    roles: string[];
	
	    static createFrom(source: any = {}) {
	        return new OperatorView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.operatorId = source["operatorId"];
	        this.roles = source["roles"];
	    }
	}
	export class RBACMutationResult {
	    name: string;
	    ruleId: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new RBACMutationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.ruleId = source["ruleId"];
	        this.status = source["status"];
	    }
	}
	export class RBACOperatorView {
	    id: string;
	    certFingerprint: string;
	    roles: string[];
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new RBACOperatorView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.certFingerprint = source["certFingerprint"];
	        this.roles = source["roles"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class RBACRoleCreateRequest {
	    name: string;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new RBACRoleCreateRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	    }
	}
	export class RBACRoleDeleteRequest {
	    name: string;
	    confirmation: string;
	
	    static createFrom(source: any = {}) {
	        return new RBACRoleDeleteRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.confirmation = source["confirmation"];
	    }
	}
	export class RBACRuleView {
	    id: string;
	    roleName: string;
	    method: string;
	    pathPattern: string;
	
	    static createFrom(source: any = {}) {
	        return new RBACRuleView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.roleName = source["roleName"];
	        this.method = source["method"];
	        this.pathPattern = source["pathPattern"];
	    }
	}
	export class RBACRoleView {
	    name: string;
	    description: string;
	    builtIn: boolean;
	    rules: RBACRuleView[];
	
	    static createFrom(source: any = {}) {
	        return new RBACRoleView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.builtIn = source["builtIn"];
	        this.rules = this.convertValues(source["rules"], RBACRuleView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RBACRuleAddRequest {
	    roleName: string;
	    method: string;
	    pathPattern: string;
	
	    static createFrom(source: any = {}) {
	        return new RBACRuleAddRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.roleName = source["roleName"];
	        this.method = source["method"];
	        this.pathPattern = source["pathPattern"];
	    }
	}
	export class RBACRuleRemoveRequest {
	    roleName: string;
	    ruleId: string;
	    confirmation: string;
	
	    static createFrom(source: any = {}) {
	        return new RBACRuleRemoveRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.roleName = source["roleName"];
	        this.ruleId = source["ruleId"];
	        this.confirmation = source["confirmation"];
	    }
	}
	
	export class ReadExportSaveResult {
	    status: string;
	    path?: string;
	    sizeBytes?: number;
	
	    static createFrom(source: any = {}) {
	        return new ReadExportSaveResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.path = source["path"];
	        this.sizeBytes = source["sizeBytes"];
	    }
	}
	
	
	export class SecretLifecycleRequest {
	    name: string;
	    version: string;
	    confirmation: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretLifecycleRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.confirmation = source["confirmation"];
	    }
	}
	export class SecretRolloutView {
	    fleet: string;
	    resourceAddress: string;
	    purpose: string;
	    risk: string;
	    effectiveHash: string;
	    changeRequestId: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretRolloutView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fleet = source["fleet"];
	        this.resourceAddress = source["resourceAddress"];
	        this.purpose = source["purpose"];
	        this.risk = source["risk"];
	        this.effectiveHash = source["effectiveHash"];
	        this.changeRequestId = source["changeRequestId"];
	    }
	}
	export class SecretUploadRequest {
	    name: string;
	    scopeType: string;
	    scopeId: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretUploadRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.scopeType = source["scopeType"];
	        this.scopeId = source["scopeId"];
	    }
	}
	export class SecretVersionView {
	    name: string;
	    version: string;
	    fingerprint: string;
	    scopeType: string;
	    scopeId: string;
	    status: string;
	    activationGeneration: number;
	    createdAt: string;
	    createdBy: string;
	    activatedAt: string;
	    activatedBy: string;
	    revokedAt: string;
	    revokedBy: string;
	    resolutionBlocked: boolean;
	    endpointCopyStatus: string;
	    rollouts: SecretRolloutView[];
	
	    static createFrom(source: any = {}) {
	        return new SecretVersionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.fingerprint = source["fingerprint"];
	        this.scopeType = source["scopeType"];
	        this.scopeId = source["scopeId"];
	        this.status = source["status"];
	        this.activationGeneration = source["activationGeneration"];
	        this.createdAt = source["createdAt"];
	        this.createdBy = source["createdBy"];
	        this.activatedAt = source["activatedAt"];
	        this.activatedBy = source["activatedBy"];
	        this.revokedAt = source["revokedAt"];
	        this.revokedBy = source["revokedBy"];
	        this.resolutionBlocked = source["resolutionBlocked"];
	        this.endpointCopyStatus = source["endpointCopyStatus"];
	        this.rollouts = this.convertValues(source["rollouts"], SecretRolloutView);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class SetupApplicationView {
	    name: string;
	    version: string;
	    platform: string;
	    architecture: string;
	
	    static createFrom(source: any = {}) {
	        return new SetupApplicationView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.platform = source["platform"];
	        this.architecture = source["architecture"];
	    }
	}
	export class SetupMaintenanceView {
	    application: SetupApplicationView;
	    standardConfigPath: string;
	    desktopProfilesPath: string;
	    profiles: ConnectionProfile[];
	
	    static createFrom(source: any = {}) {
	        return new SetupMaintenanceView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.application = this.convertValues(source["application"], SetupApplicationView);
	        this.standardConfigPath = source["standardConfigPath"];
	        this.desktopProfilesPath = source["desktopProfilesPath"];
	        this.profiles = this.convertValues(source["profiles"], ConnectionProfile);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	
	export class WorkspaceSections {
	    fleets: SectionResult;
	    endpoints: SectionResult;
	    state: SectionResult;
	    changeRequests: SectionResult;
	    activity: SectionResult;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceSections(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fleets = this.convertValues(source["fleets"], SectionResult);
	        this.endpoints = this.convertValues(source["endpoints"], SectionResult);
	        this.state = this.convertValues(source["state"], SectionResult);
	        this.changeRequests = this.convertValues(source["changeRequests"], SectionResult);
	        this.activity = this.convertValues(source["activity"], SectionResult);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WorkspaceView {
	    operator: OperatorView;
	    sections: WorkspaceSections;
	    endpoints: EndpointRow[];
	    fleets: FleetSummary[];
	    stateEvidence: StateEvidence[];
	    changeRequests: ChangeRequestSummary[];
	    activity: ActivityEvent[];
	    activityNextCursor: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.operator = this.convertValues(source["operator"], OperatorView);
	        this.sections = this.convertValues(source["sections"], WorkspaceSections);
	        this.endpoints = this.convertValues(source["endpoints"], EndpointRow);
	        this.fleets = this.convertValues(source["fleets"], FleetSummary);
	        this.stateEvidence = this.convertValues(source["stateEvidence"], StateEvidence);
	        this.changeRequests = this.convertValues(source["changeRequests"], ChangeRequestSummary);
	        this.activity = this.convertValues(source["activity"], ActivityEvent);
	        this.activityNextCursor = source["activityNextCursor"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}
