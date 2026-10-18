export namespace main {
	
	export class DestinationImage {
	    dataUrl: string;
	    caption: string;
	    pageUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new DestinationImage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dataUrl = source["dataUrl"];
	        this.caption = source["caption"];
	        this.pageUrl = source["pageUrl"];
	    }
	}
	export class PhotoResult {
	    photos: Record<string, Array<string>>;
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new PhotoResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.photos = source["photos"];
	        this.warnings = source["warnings"];
	    }
	}
	export class Species {
	    comName: string;
	    sciName?: string;
	    speciesCode?: string;
	    frequency?: number;
	    notes?: string;
	    Habitat?: string;
	    taxonOrder?: number;
	    seenBefore?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Species(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.comName = source["comName"];
	        this.sciName = source["sciName"];
	        this.speciesCode = source["speciesCode"];
	        this.frequency = source["frequency"];
	        this.notes = source["notes"];
	        this.Habitat = source["Habitat"];
	        this.taxonOrder = source["taxonOrder"];
	        this.seenBefore = source["seenBefore"];
	    }
	}
	export class Quiz {
	    name: string;
	    path: string;
	    start_month: number;
	    end_month: number;
	    location: string;
	    notes: string;
	    species: Species[];
	    taxonomyWarning?: string;
	
	    static createFrom(source: any = {}) {
	        return new Quiz(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.start_month = source["start_month"];
	        this.end_month = source["end_month"];
	        this.location = source["location"];
	        this.notes = source["notes"];
	        this.species = this.convertValues(source["species"], Species);
	        this.taxonomyWarning = source["taxonomyWarning"];
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

