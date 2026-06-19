export namespace config {
	
	export class Config {
	    server_url: string;
	    auto_start: boolean;
	    minimize_to_tray: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.server_url = source["server_url"];
	        this.auto_start = source["auto_start"];
	        this.minimize_to_tray = source["minimize_to_tray"];
	    }
	}

}

export namespace vpn {
	
	export class PeerInfo {
	    id: string;
	    public_key: string;
	    virtual_ip: string;
	    endpoint?: string;
	    connected: boolean;
	    // Go type: time
	    last_seen: any;
	    latency_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new PeerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.public_key = source["public_key"];
	        this.virtual_ip = source["virtual_ip"];
	        this.endpoint = source["endpoint"];
	        this.connected = source["connected"];
	        this.last_seen = this.convertValues(source["last_seen"], null);
	        this.latency_ms = source["latency_ms"];
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

