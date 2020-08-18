/* Do not change, this code is generated from Golang structs */


export class Address {
    city: string;
    number: number;
    country?: string;

    static createFrom(source: any) {
        if ('string' === typeof source) source = JSON.parse(source);
        const result = new Address();
        result.city = source["city"];
        result.number = source["number"];
        result.country = source["country"];
        return result;
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

    static createFrom(source: any) {
        if ('string' === typeof source) source = JSON.parse(source);
        const result = new PersonalInfo();
        result.hobby = source["hobby"];
        result.pet_name = source["pet_name"];
        return result;
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
    address: Address;
    metadata: {[key:string]:string};
    friends: Person[];

    static createFrom(source: any) {
        if ('string' === typeof source) source = JSON.parse(source);
        const result = new Person();
        result.name = source["name"];
        result.personal_info = PersonalInfo.createFrom(source["personal_info"]);
        result.nicknames = source["nicknames"];
        result.addresses = source["addresses"] && source["addresses"].map(function(element: any) { return Address.createFrom(element); });
        result.address = Address.createFrom(source["address"]);
        result.metadata = source["metadata"];
        result.friends = source["friends"] && source["friends"].map(function(element: any) { return Person.createFrom(element); });
        return result;
    }

    //[Person:]

    getInfo = () => {
        return "name:" + this.name;
    }

    //[end]
}