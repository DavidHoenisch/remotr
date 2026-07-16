export namespace main {
	
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

}

