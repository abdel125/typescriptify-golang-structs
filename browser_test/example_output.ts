/* Do not change, this code is generated from Golang structs */


export class Address {
    city: string;
    number: number;
    country?: string;

    static createFrom(source: any = {}) {
        return new Address(source);
    }

    constructor(source: any = {}) {
        if ('string' === typeof source) source = JSON.parse(source);
        this.city = source["city"];
        this.number = source["number"];
        this.country = source["country"];
    }
    //[Address:]
    /* Custom code here */

    getAddressString = () => {
        return this.city + " " + this.number;
    }

    //[end]
}
export class PersonalInfo {
    hobby: string[];
    pet_name: string;

    static createFrom(source: any = {}) {
        return new PersonalInfo(source);
    }

    constructor(source: any = {}) {
        if ('string' === typeof source) source = JSON.parse(source);
        this.hobby = source["hobby"];
        this.pet_name = source["pet_name"];
    }
    //[PersonalInfo:]

    getPersonalInfoString = () => {
        return "pet:" + this.pet_name;
    }

    //[end]
}
export class Person {
    name: string;
    personal_info: PersonalInfo;
    nicknames: string[];
    addresses: Address[];
    address?: Address;
    metadata: {[key:string]:string};
    friends: Person[];

    static createFrom(source: any = {}) {
        return new Person(source);
    }

    constructor(source: any = {}) {
        if ('string' === typeof source) source = JSON.parse(source);
        this.name = source["name"];
        this.personal_info = this.convertValues(source["personal_info"], PersonalInfo);
        this.nicknames = source["nicknames"];
        this.addresses = this.convertValues(source["addresses"], Address);
        this.address = this.convertValues(source["address"], Address);
        this.metadata = source["metadata"];
        this.friends = this.convertValues(source["friends"], Person);
    }

	convertValues(a: any, classs: any, asMap: boolean = false): any {
		if (!a) {
			return a;
		}
		if (a.slice) {
			return (a as any[]).map(elem => (this.convertValues || eval("convertValues"))(elem, classs));
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
    //[Person:]

    getInfo = () => {
        return "name:" + this.name;
    }

    //[end]
}