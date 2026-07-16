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

}

