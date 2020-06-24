// Copyright (c) 2020 Sorint.lab S.p.A.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package database

import (
	"context"
	"regexp"
	"time"

	"github.com/amreo/mu"
	"github.com/ercole-io/ercole/model"
	"github.com/ercole-io/ercole/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SearchHosts search hosts
func (md *MongoDatabase) SearchHosts(mode string, keywords []string, otherFilters SearchHostsFilters, sortBy string, sortDesc bool, page int, pageSize int, location string, environment string, olderThan time.Time) ([]map[string]interface{}, utils.AdvancedErrorInterface) {
	var out []map[string]interface{} = make([]map[string]interface{}, 0)

	//Find the matching hostdata
	cur, err := md.Client.Database(md.Config.Mongodb.DBName).Collection("hosts").Aggregate(
		context.TODO(),
		mu.MAPipeline(
			FilterByLocationAndEnvironmentSteps(location, environment),
			FilterByOldnessSteps(olderThan),
			mu.MAPipeline(
				mu.APOptionalStage(otherFilters.Hostname != "", mu.APMatch(bson.M{
					"Hostname": primitive.Regex{Pattern: regexp.QuoteMeta(otherFilters.Hostname), Options: "i"},
				})),
				mu.APOptionalStage(otherFilters.Database != "", mu.APMatch(bson.M{
					"Features.Oracle.Database.Databases.Name": primitive.Regex{Pattern: regexp.QuoteMeta(otherFilters.Database), Options: "i"},
				})),
				mu.APOptionalStage(otherFilters.HardwareAbstractionTechnology != "", mu.APMatch(bson.M{
					"Info.HardwareAbstractionTechnology": primitive.Regex{Pattern: regexp.QuoteMeta(otherFilters.HardwareAbstractionTechnology), Options: "i"},
				})),
				mu.APOptionalStage(otherFilters.OperatingSystem != "", mu.APMatch(bson.M{
					"Info.OS": primitive.Regex{Pattern: regexp.QuoteMeta(otherFilters.OperatingSystem), Options: "i"},
				})),
				mu.APOptionalStage(otherFilters.Kernel != "", mu.APMatch(bson.M{
					"Info.Kernel": primitive.Regex{Pattern: regexp.QuoteMeta(otherFilters.Kernel), Options: "i"},
				})),
				mu.APOptionalStage(otherFilters.LTEMemoryTotal != -1, mu.APMatch(bson.M{
					"Info.MemoryTotal": mu.QOLessThanOrEqual(otherFilters.LTEMemoryTotal),
				})),
				mu.APOptionalStage(otherFilters.GTEMemoryTotal != -1, mu.APMatch(bson.M{
					"Info.MemoryTotal": bson.M{
						"$gte": otherFilters.GTEMemoryTotal,
					},
				})),
				mu.APOptionalStage(otherFilters.LTESwapTotal != -1, mu.APMatch(bson.M{
					"Info.SwapTotal": mu.QOLessThanOrEqual(otherFilters.LTESwapTotal),
				})),
				mu.APOptionalStage(otherFilters.GTESwapTotal != -1, mu.APMatch(bson.M{
					"Info.SwapTotal": bson.M{
						"$gte": otherFilters.GTESwapTotal,
					},
				})),
				getIsMemberOfClusterFilterStep(otherFilters.IsMemberOfCluster),
				mu.APOptionalStage(otherFilters.CPUModel != "", mu.APMatch(bson.M{
					"Info.CPUModel": primitive.Regex{Pattern: regexp.QuoteMeta(otherFilters.CPUModel), Options: "i"},
				})),
				mu.APOptionalStage(otherFilters.LTECPUCores != -1, mu.APMatch(bson.M{
					"Info.CPUCores": mu.QOLessThanOrEqual(otherFilters.LTECPUCores),
				})),
				mu.APOptionalStage(otherFilters.GTECPUCores != -1, mu.APMatch(bson.M{
					"Info.CPUCores": bson.M{
						"$gte": otherFilters.GTECPUCores,
					},
				})),
				mu.APOptionalStage(otherFilters.LTECPUThreads != -1, mu.APMatch(bson.M{
					"Info.CPUThreads": mu.QOLessThanOrEqual(otherFilters.LTECPUThreads),
				})),
				mu.APOptionalStage(otherFilters.GTECPUThreads != -1, mu.APMatch(bson.M{
					"Info.CPUThreads": bson.M{
						"$gte": otherFilters.GTECPUThreads,
					},
				})),
			),
			mu.APSearchFilterStage([]interface{}{
				"$Hostname",
				"$Features.Oracle.Database.Databases.Name",
				"$Features.Oracle.Database.UniqueName",
				"$Clusters.Name",
			}, keywords),
			mu.APOptionalStage(mode != "mongo", mu.MAPipeline(
				mu.APOptionalStage(mode == "lms", mu.APMatch(
					mu.QOExpr(mu.APOGreater(mu.APOSize(mu.APOIfNull("$Features.Oracle.Database.Databases", bson.A{})), 0))),
				),
				AddAssociatedClusterNameAndVirtualizationNode(olderThan),
				mu.MAPipeline(
					getClusterFilterStep(otherFilters.Cluster),
					mu.APOptionalStage(otherFilters.VirtualizationNode != "", mu.APMatch(bson.M{
						"VirtualizationNode": primitive.Regex{Pattern: regexp.QuoteMeta(otherFilters.VirtualizationNode), Options: "i"},
					})),
				),
				mu.APOptionalStage(mode == "summary", mu.APProject(bson.M{
					"Hostname":                      true,
					"Location":                      true,
					"Environment":                   true,
					"Cluster":                       true,
					"AgentVersion":                  true,
					"VirtualizationNode":            true,
					"CreatedAt":                     true,
					"OS":                            mu.APOConcat("$Info.OS", " ", "$Info.OSVersion"),
					"Kernel":                        mu.APOConcat("$Info.Kernel", " ", "$Info.KernelVersion"),
					"OracleClusterware":             "$ClusterMembershipStatus.OracleClusterware",
					"VeritasClusterServer":          "$ClusterMembershipStatus.VeritasClusterServer",
					"SunCluster":                    "$ClusterMembershipStatus.SunCluster",
					"HACMP":                         "$ClusterMembershipStatus.HACMP",
					"HardwareAbstraction":           "$Info.HardwareAbstraction",
					"HardwareAbstractionTechnology": "$Info.HardwareAbstractionTechnology",
					"CPUThreads":                    "$Info.CPUThreads",
					"CPUCores":                      "$Info.CPUCores",
					"CPUSockets":                    "$Info.CPUSockets",
					"MemTotal":                      "$Info.MemoryTotal",
					"SwapTotal":                     "$Info.SwapTotal",
					"CPUModel":                      "$Info.CPUModel",
				})),
				mu.APOptionalStage(mode == "lms", mu.MAPipeline(
					mu.APMatch(mu.QOExpr(mu.APOGreater(mu.APOSize("$Features.Oracle.Database.Databases"), 0))),
					mu.APSet(bson.M{
						"Database": mu.APOArrayElemAt("$Features.Oracle.Database.Databases", 0),
					}),
					mu.APUnset("Extra"),
					mu.APSet(bson.M{
						"VmwareOrOVM": mu.APOOr(mu.APOEqual("$Info.HardwareAbstractionTechnology", "VMWARE"), mu.APOEqual("$Info.HardwareAbstractionPlatform", "OVM")),
					}),
					mu.APProject(bson.M{
						"PhysicalServerName":       mu.APOCond("$VmwareOrOVM", mu.APOIfNull("$Cluster", ""), "$Hostname"),
						"VirtualServerName":        mu.APOCond("$VmwareOrOVM", "$Hostname", mu.APOIfNull("$Cluster", "")),
						"VirtualizationTechnology": "$Info.HardwareAbstractionTechnology",
						"DBInstanceName":           "$Database.Name",
						"PluggableDatabaseName":    "",
						"ConnectString":            "",
						"ProductVersion":           mu.APOArrayElemAt(mu.APOSplit("$Database.Version", "."), 0),
						"ProductEdition":           mu.APOArrayElemAt(mu.APOSplit("$Database.Version", " "), 1),
						"Environment":              "$Environment",
						"Features": mu.APOJoin(mu.APOMap(
							mu.APOFilter("$Database.Licenses", "lic", mu.APOAnd(mu.APOGreater("$$lic.Count", 0), mu.APONotEqual("$$lic.Name", "Oracle STD"), mu.APONotEqual("$$lic.Name", "Oracle EXE"), mu.APONotEqual("$$lic.Name", "Oracle ENT"))),
							"lic",
							"$$lic.Name",
						), ", "),
						"RacNodeNames":   "",
						"ProcessorModel": "$Info.CPUModel",
						"Processors":     "$Info.CPUSockets",
						"CoresPerProcessor": mu.APOCond(
							mu.APOAnd(
								mu.APOGreaterOrEqual("$Info.CPUCores", "$Info.CPUSockets"),
								mu.APONotEqual("$Info.CPUSockets", 0),
							),
							mu.APODivide("$Info.CPUCores", "$Info.CPUSockets"),
							"$Info.CPUCores",
						),
						"ThreadsPerCore": mu.APOCond(
							mu.APOGreaterOrEqual(mu.APOIndexOfCp("$Info.CPUModel", "SPARC"), 0),
							8,
							2,
						),
						"ProcessorSpeed":     "$Info.CPUFrequency",
						"ServerPurchaseDate": "",
						"OperatingSystem":    mu.APOConcat("$Info.OS", " ", "$Info.OSVersion"),
						"Notes":              "",
					}),
					mu.APSet(bson.M{
						"PhysicalCores": mu.APOCond(mu.APOEqual("$Info.CPUSockets", 0), "$CoresPerProcessor", bson.M{
							"$multiply": bson.A{"$CoresPerProcessor", "$Processors"},
						}),
					}),
				)),
				mu.APOptionalSortingStage(sortBy, sortDesc),
				mu.APOptionalPagingStage(page, pageSize),
			)),
		),
	)
	if err != nil {
		return nil, utils.NewAdvancedErrorPtr(err, "DB ERROR")
	}

	//Decode the documents
	for cur.Next(context.TODO()) {
		var item map[string]interface{}
		if cur.Decode(&item) != nil {
			return nil, utils.NewAdvancedErrorPtr(err, "Decode ERROR")
		}
		out = append(out, item)
	}
	return out, nil
}

func getClusterFilterStep(cl *string) interface{} {
	if cl == nil {
		return mu.APMatch(bson.M{
			"Cluster": nil,
		})
	} else if *cl != "" {
		return mu.APMatch(bson.M{
			"Cluster": primitive.Regex{Pattern: regexp.QuoteMeta(*cl), Options: "i"},
		})
	} else {
		return bson.A{}
	}
}

func getIsMemberOfClusterFilterStep(member *bool) interface{} {
	if member != nil {
		return mu.APMatch(mu.QOExpr(
			mu.APOEqual(*member, mu.APOOr("$ClusterMembershipStatus.OracleClusterware", "$ClusterMembershipStatus.VeritasClusterServer", "$ClusterMembershipStatus.SunCluster", "$ClusterMembershipStatus.HACMP")),
		))
	}
	return bson.A{}
}

// GetHost fetch all informations about a host in the database
func (md *MongoDatabase) GetHost(hostname string, olderThan time.Time, raw bool) (interface{}, utils.AdvancedErrorInterface) {
	var out map[string]interface{}

	//Find the matching hostdata
	cur, err := md.Client.Database(md.Config.Mongodb.DBName).Collection("hosts").Aggregate(
		context.TODO(),
		mu.MAPipeline(
			FilterByOldnessSteps(olderThan),
			mu.APMatch(bson.M{
				"Hostname": hostname,
			}),
			mu.APOptionalStage(!raw, mu.MAPipeline(
				mu.APLookupPipeline("alerts", bson.M{"hn": "$Hostname"}, "Alerts", mu.MAPipeline(
					mu.APMatch(mu.QOExpr(mu.APOEqual("$OtherInfo.Hostname", "$$hn"))),
				)),
				AddAssociatedClusterNameAndVirtualizationNode(olderThan),
				mu.APLookupPipeline(
					"hosts",
					bson.M{
						"hn": "$Hostname",
						"ca": "$CreatedAt",
					},
					"History",
					mu.MAPipeline(
						mu.APMatch(mu.QOExpr(mu.APOAnd(mu.APOEqual("$Hostname", "$$hn"), mu.APOGreaterOrEqual("$$ca", "$CreatedAt")))),
						mu.APProject(bson.M{
							"CreatedAt": 1,
							"Features.Oracle.Database.Databases.Name":          1,
							"Features.Oracle.Database.Databases.DatafileSize":  1,
							"Features.Oracle.Database.Databases.SegmentsSize":  1,
							"Features.Oracle.Database.Databases.DailyCPUUsage": 1,
							"TotalDailyCPUUsage":                               mu.APOSumReducer("$Features.Oracle.Database.Databases", mu.APOConvertToDoubleOrZero("$$this.DailyCPUUsage")),
						}),
					),
				),
				mu.APSet(bson.M{
					"Features.Oracle.Database.Databases": mu.APOMap(
						"$Features.Oracle.Database.Databases",
						"db",
						mu.APOMergeObjects(
							"$$db",
							bson.M{
								"Changes": mu.APOFilter(
									mu.APOMap("$History", "hh", mu.APOMergeObjects(
										bson.M{"Updated": "$$hh.CreatedAt"},
										mu.APOArrayElemAt(mu.APOFilter("$$hh.Features.Oracle.Database.Databases", "hdb", mu.APOEqual("$$hdb.Name", "$$db.Name")), 0),
									)),
									"time_frame",
									"$$time_frame.SegmentsSize",
								),
							},
						),
					),
				}),
				mu.APUnset(
					"Features.Oracle.Database.Databases.Changes.Name",
					"History.Features",
				),
			)),
		),
	)
	if err != nil {
		return nil, utils.NewAdvancedErrorPtr(err, "DB ERROR")
	}

	//Next the cursor. If there is no document return a empty document
	hasNext := cur.Next(context.TODO())
	if !hasNext {
		return nil, utils.AerrHostNotFound
	}

	//Decode the document
	if err := cur.Decode(&out); err != nil {
		return nil, utils.NewAdvancedErrorPtr(err, "DB ERROR")
	}

	return out, nil
}

// ListLocations list locations
func (md *MongoDatabase) ListLocations(location string, environment string, olderThan time.Time) ([]string, utils.AdvancedErrorInterface) {
	var out []string = make([]string, 0)

	//Find the matching hostdata
	cur, err := md.Client.Database(md.Config.Mongodb.DBName).Collection("hosts").Aggregate(
		context.TODO(),
		mu.MAPipeline(
			FilterByOldnessSteps(olderThan),
			FilterByLocationAndEnvironmentSteps(location, environment),
			mu.APGroup(bson.M{
				"_id": "$Location",
			}),
		),
	)
	if err != nil {
		return nil, utils.NewAdvancedErrorPtr(err, "DB ERROR")
	}

	//Decode the documents
	for cur.Next(context.TODO()) {
		var item map[string]string
		if cur.Decode(&item) != nil {
			return nil, utils.NewAdvancedErrorPtr(err, "Decode ERROR")
		}
		out = append(out, item["_id"])
	}
	return out, nil
}

// ListEnvironments list environments
func (md *MongoDatabase) ListEnvironments(location string, environment string, olderThan time.Time) ([]string, utils.AdvancedErrorInterface) {
	var out []string = make([]string, 0)

	//Find the matching hostdata
	cur, err := md.Client.Database(md.Config.Mongodb.DBName).Collection("hosts").Aggregate(
		context.TODO(),
		mu.MAPipeline(
			FilterByOldnessSteps(olderThan),
			FilterByLocationAndEnvironmentSteps(location, environment),
			mu.APGroup(bson.M{
				"_id": "$Environment",
			}),
		),
	)
	if err != nil {
		return nil, utils.NewAdvancedErrorPtr(err, "DB ERROR")
	}

	//Decode the documents
	for cur.Next(context.TODO()) {
		var item map[string]string
		if cur.Decode(&item) != nil {
			return nil, utils.NewAdvancedErrorPtr(err, "Decode ERROR")
		}
		out = append(out, item["_id"])
	}
	return out, nil
}

// FindHostData find the current hostdata with a certain hostname
func (md *MongoDatabase) FindHostData(hostname string) (model.HostDataBE, utils.AdvancedErrorInterface) {
	//Find the hostdata
	res := md.Client.Database(md.Config.Mongodb.DBName).Collection("hosts").FindOne(context.TODO(), bson.M{
		"Hostname": hostname,
		"Archived": false,
	})
	if res.Err() == mongo.ErrNoDocuments {
		return model.HostDataBE{}, utils.AerrHostNotFound
	} else if res.Err() != nil {
		return model.HostDataBE{}, utils.NewAdvancedErrorPtr(res.Err(), "DB ERROR")
	}

	//Decode the data
	var out model.HostDataBE
	if err := res.Decode(&out); err != nil {
		return model.HostDataBE{}, utils.NewAdvancedErrorPtr(res.Err(), "DB ERROR")
	}

	var out2 map[string]interface{}
	if err := res.Decode(&out2); err != nil {
		// return model.HostDataBE{}, utils.NewAdvancedErrorPtr(res.Err(), "DB ERROR")
	}

	//Return it!
	return out, nil
}

// ReplaceHostData adds a new hostdata to the database
func (md *MongoDatabase) ReplaceHostData(hostData model.HostDataBE) utils.AdvancedErrorInterface {
	_, err := md.Client.Database(md.Config.Mongodb.DBName).Collection("hosts").ReplaceOne(context.TODO(),
		bson.M{
			"_id": hostData.ID,
		},
		hostData,
	)
	if err != nil {
		return utils.NewAdvancedErrorPtr(err, "DB ERROR")
	}
	return nil
}

// ExistHostdata return true if exist a non-archived hostdata with the hostname equal hostname
func (md *MongoDatabase) ExistHostdata(hostname string) (bool, utils.AdvancedErrorInterface) {
	//Count the number of new NO_DATA alerts associated to the host
	val, err := md.Client.Database(md.Config.Mongodb.DBName).Collection("hosts").CountDocuments(context.TODO(), bson.M{
		"Archived": false,
		"Hostname": hostname,
	}, &options.CountOptions{
		Limit: utils.Intptr(1),
	})
	if err != nil {
		return false, utils.NewAdvancedErrorPtr(err, "DB ERROR")
	}

	//Return true if the count > 0
	return val > 0, nil
}

// ArchiveHost archive the specified host
func (md *MongoDatabase) ArchiveHost(hostname string) utils.AdvancedErrorInterface {
	if _, err := md.Client.Database(md.Config.Mongodb.DBName).Collection("hosts").UpdateOne(context.TODO(), bson.M{
		"Hostname": hostname,
		"Archived": false,
	}, mu.UOSet(bson.M{
		"Archived": true,
	})); err != nil {
		return utils.NewAdvancedErrorPtr(err, "DB ERROR")
	} else {
		return nil
	}
}
