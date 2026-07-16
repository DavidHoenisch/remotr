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

