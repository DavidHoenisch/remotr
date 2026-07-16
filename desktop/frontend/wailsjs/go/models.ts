export namespace main {
	
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
	export class EndpointRow {
	    endpointId: string;
	    fleet: string;
	    usernames: string[];
	    compliance: string;
	    freshness: string;
	    desiredAgentVersion: string;
	    reportedAgentVersion: string;
	    releaseRef: string;
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

