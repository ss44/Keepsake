export namespace brightwheel {
	
	export class Student {
	    object_id: string;
	    first_name: string;
	    last_name: string;
	
	    static createFrom(source: any = {}) {
	        return new Student(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.object_id = source["object_id"];
	        this.first_name = source["first_name"];
	        this.last_name = source["last_name"];
	    }
	}

}

export namespace library {
	
	export class File {
	    path: string;
	    name: string;
	    size: number;
	    mod_time: number;
	    is_video: boolean;
	
	    static createFrom(source: any = {}) {
	        return new File(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.size = source["size"];
	        this.mod_time = source["mod_time"];
	        this.is_video = source["is_video"];
	    }
	}

}

export namespace main {
	
	export class RemoteMediaItem {
	    url: string;
	    student_id: string;
	    expected_name: string;
	    is_video: boolean;
	    is_more_indicator?: boolean;
	    more_count?: number;
	
	    static createFrom(source: any = {}) {
	        return new RemoteMediaItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.student_id = source["student_id"];
	        this.expected_name = source["expected_name"];
	        this.is_video = source["is_video"];
	        this.is_more_indicator = source["is_more_indicator"];
	        this.more_count = source["more_count"];
	    }
	}

}

