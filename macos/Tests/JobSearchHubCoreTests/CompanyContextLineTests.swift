import Foundation
import JobSearchHubCore
import Testing

/// A made-up company with the given facts, decoded as the hub sends it.
private func makeCompany(industry: String? = nil, country: String? = nil, size: String? = nil) throws -> Company {
    var fields: [String: String] = [
        "id": "0aa55565-58d2-4247-ba01-cba65060a316", "name": "Northwind", "domain": "northwind.example",
        "created_at": "2026-09-28T12:00:00Z", "updated_at": "2026-09-28T12:00:00Z",
    ]
    fields["industry"] = industry
    fields["headquarters_country"] = country
    fields["employee_count_range"] = size
    return try HubJSON.makeDecoder().decode(Company.self, from: JSONEncoder().encode(fields))
}

@Test func aCompanysLineReadsItsIndustryAndSize() throws {
    #expect(try makeCompany(industry: "Payments", country: "Canada", size: "51-200").contextLine == "Payments · 51-200 people")
    #expect(try makeCompany(industry: "Health data").contextLine == "Health data")
}

@Test func aCompanysLineFallsBackToItsCountryThenItsDomain() throws {
    #expect(try makeCompany(country: "Canada", size: "51-200").contextLine == "Canada · 51-200 people")
    #expect(try makeCompany(industry: "", country: "Canada").contextLine == "Canada")
    #expect(try makeCompany(size: "51-200").contextLine == "51-200 people")
    #expect(try makeCompany().contextLine == "northwind.example")
    #expect(try makeCompany(industry: "", country: "", size: "").contextLine == "northwind.example")
}
