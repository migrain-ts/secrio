export namespace desktop {
	
	export class CopyResult {
	    clearInSeconds: number;
	
	    static createFrom(source: any = {}) {
	        return new CopyResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.clearInSeconds = source["clearInSeconds"];
	    }
	}
	export class VaultDTO {
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new VaultDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class CurrentVaultDTO {
	    vault: VaultDTO;
	    unlocked: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CurrentVaultDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.vault = this.convertValues(source["vault"], VaultDTO);
	        this.unlocked = source["unlocked"];
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
	export class EnvDTO {
	    id: string;
	    name: string;
	    secretCount: number;
	
	    static createFrom(source: any = {}) {
	        return new EnvDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.secretCount = source["secretCount"];
	    }
	}
	export class ImportPreview {
	    new: string[];
	    duplicate: string[];
	
	    static createFrom(source: any = {}) {
	        return new ImportPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.new = source["new"];
	        this.duplicate = source["duplicate"];
	    }
	}
	export class ProjectDTO {
	    id: string;
	    name: string;
	    envCount: number;
	    secretCount: number;
	
	    static createFrom(source: any = {}) {
	        return new ProjectDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.envCount = source["envCount"];
	        this.secretCount = source["secretCount"];
	    }
	}
	export class SecretDTO {
	    name: string;
	    type: string;
	    description: string;
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.description = source["description"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}

}

