package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"

	"github.com/ercole-io/ercole/v2/config"
	"github.com/ercole-io/ercole/v2/model"
	"github.com/ercole-io/ercole/v2/utils"
)

func (hds *HostDataService) oracleDatabasesChecks(previousHostdata, hostdata *model.HostDataBE) {
	oracleFeature := hostdata.Features.Oracle

	if oracleFeature.Database == nil || oracleFeature.Database.Databases == nil {
		return
	}

	hds.checkSecondaryDbs(hostdata)

	hds.setLicenseTypes(hostdata)

	if err := hds.checkNewLicenses(previousHostdata, hostdata); err != nil {
		hds.Log.Error(err)
		return
	}

	for _, dbname := range hostdata.Features.Oracle.Database.UnlistedRunningDatabases {
		if err := hds.throwUnlistedRunningDatabasesAlert(dbname, hostdata.Hostname); err != nil {
			hds.Log.Error(err)
		}
	}
}

func (hds *HostDataService) checkSecondaryDbs(hostdata *model.HostDataBE) {
	for i := range hostdata.Features.Oracle.Database.Databases {
		db := &hostdata.Features.Oracle.Database.Databases[i]

		if db.Status == model.OracleDatabaseStatusMounted &&
			db.Role != model.OracleDatabaseRolePrimary {
			hds.addLicensesToSecondaryDb(hostdata.Info, db)
		}
	}
}

func (hds *HostDataService) addLicensesToSecondaryDb(hostInfo model.Host, secondaryDb *model.OracleDatabase) {
	dbs, err := hds.getPrimaryOpenOracleDatabases()
	if err != nil {
		utils.LogErr(hds.Log, utils.NewAdvancedErrorPtr(err, "INSERT_HOSTDATA_ORACLE_DATABASE"))
		return
	}

	var primaryDb *model.OracleDatabase
	for i, db := range dbs {
		if db.DbID == secondaryDb.DbID && db.Name == secondaryDb.Name {
			primaryDb = &dbs[i]
			break
		}
	}

	if primaryDb == nil {
		alert := model.Alert{
			AlertCategory:           model.AlertCategoryEngine,
			AlertAffectedTechnology: nil,
			AlertCode:               model.AlertCodeMissingPrimaryDatabase,
			AlertSeverity:           model.AlertSeverityWarning,
			AlertStatus:             model.AlertStatusNew,
			Description:             fmt.Sprintf("Missing primary database on standby database: %s", secondaryDb.Name),
			Date:                    hds.TimeNow(),
			OtherInfo: map[string]interface{}{
				"hostname": hostInfo.Hostname,
				"dbname":   secondaryDb.Name,
			},
		}

		err := hds.AlertSvcClient.ThrowNewAlert(alert)
		if err != nil {
			utils.LogErr(hds.Log, utils.NewAdvancedErrorPtr(err, "Can't throw new alert"))
			return
		}
	}

	coreFactor := secondaryDb.CoreFactor(hostInfo)

primaryDbLicensesCycle:
	for _, primaryDbLicense := range primaryDb.Licenses {

		if primaryDbLicense.Count > 0 {
			for i := range secondaryDb.Licenses {
				secondaryDbLicense := &secondaryDb.Licenses[i]

				if secondaryDbLicense.Name == primaryDbLicense.Name {
					secondaryDbLicense.Count = float64(hostInfo.CPUCores) * coreFactor
					continue primaryDbLicensesCycle
				}
			}

			secondaryDb.Licenses = append(secondaryDb.Licenses,
				model.OracleDatabaseLicense{
					LicenseTypeID: primaryDbLicense.LicenseTypeID,
					Name:          primaryDbLicense.Name,
					Count:         float64(hostInfo.CPUCores) * coreFactor,
				})
		}
	}
}

func (hds *HostDataService) getPrimaryOpenOracleDatabases() (dbs []model.OracleDatabase, err error) {
	values := url.Values{}
	values.Set("full", "true")
	url := utils.NewAPIUrl(
		hds.Config.APIService.RemoteEndpoint,
		hds.Config.APIService.AuthenticationProvider.Username,
		hds.Config.APIService.AuthenticationProvider.Password,
		"/hosts/technologies/oracle/databases", values).String()

	resp, err := http.Get(url)
	if err != nil || resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, utils.NewAdvancedErrorPtr(err, "Can't retrieve databases")
	}

	decoder := json.NewDecoder(resp.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&dbs); err != nil {
		return nil, utils.NewAdvancedErrorPtr(err, "Can't decode databases")
	}

	for i := 0; i < len(dbs); {
		db := &dbs[i]

		if db.Role == model.OracleDatabaseRolePrimary && db.Status == model.OracleDatabaseStatusOpen {
			i += 1
			continue
		}

		dbs = removeFromDBs(dbs, i)
	}

	return dbs, nil
}

// Do not mantain order
func removeFromDBs(s []model.OracleDatabase, i int) []model.OracleDatabase {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}

func licenseTypesSorter(config config.DataService, environment string, licenseTypes []model.OracleDatabaseLicenseType,
) func(int, int) bool {
	orderOfPriority, ok := config.LicenseTypeMetricsByEnvironment[environment]
	if !ok {
		orderOfPriority = config.LicenseTypeMetricsDefault
	}

	priorityMap := make(map[string]int, len(orderOfPriority))
	for i, p := range orderOfPriority {
		priorityMap[p] = len(orderOfPriority) - i
	}

	return func(i, j int) bool {
		x := &licenseTypes[i]
		y := &licenseTypes[j]

		return priorityMap[x.Metric] >= priorityMap[y.Metric]
	}
}

func setLicenseTypeIDs(licenseTypes []model.OracleDatabaseLicenseType, database *model.OracleDatabase) {
licenses:
	for i := range database.Licenses {
		license := &database.Licenses[i]

		for _, licenseType := range licenseTypes {
			for _, alias := range licenseType.Aliases {
				if alias == license.Name {
					license.LicenseTypeID = licenseType.ID
					continue licenses
				}
			}
		}
	}
}

func (hds *HostDataService) setLicenseTypes(hostdata *model.HostDataBE) {
	licenseTypes, err := hds.getOracleDatabaseLicenseTypes(hostdata.Environment)
	if err != nil {
		utils.LogErr(hds.Log, utils.NewAdvancedErrorPtr(err, "INSERT_HOSTDATA_ORACLE_DATABASE"))
		licenseTypes = make([]model.OracleDatabaseLicenseType, 0)
	}

	for i := range hostdata.Features.Oracle.Database.Databases {
		db := &hostdata.Features.Oracle.Database.Databases[i]
		setLicenseTypeIDs(licenseTypes, db)
	}
}

func (hds *HostDataService) getOracleDatabaseLicenseTypes(environment string,
) ([]model.OracleDatabaseLicenseType, error) {
	url := utils.NewAPIUrlNoParams(
		hds.Config.APIService.RemoteEndpoint,
		hds.Config.APIService.AuthenticationProvider.Username,
		hds.Config.APIService.AuthenticationProvider.Password,
		"settings/oracle/database/license-types").String()

	resp, err := http.Get(url)
	if err != nil || resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, utils.NewAdvancedErrorPtr(err, "Can't retrieve licenseTypes")
	}

	licenseTypes := make([]model.OracleDatabaseLicenseType, 0)

	decoder := json.NewDecoder(resp.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&licenseTypes); err != nil {
		return nil, utils.NewAdvancedErrorPtr(err, "Can't decode licenseTypes")
	}

	sort.Slice(licenseTypes, licenseTypesSorter(hds.Config.DataService, environment, licenseTypes))

	return licenseTypes, nil
}

// find the difference between the data and generate eventually alerts for such difference
func (hds *HostDataService) checkNewLicenses(previous, new *model.HostDataBE) error {
	previousDbs := make(map[string]model.OracleDatabase)
	if previous.Features.Oracle.Database != nil && previous.Features.Oracle.Database.Databases != nil {
		previousDbs = model.DatabasesArrayAsMap(previous.Features.Oracle.Database.Databases)
	}

	newDbs := make(map[string]model.OracleDatabase)
	if new.Features.Oracle.Database != nil && new.Features.Oracle.Database.Databases != nil {
		newDbs = model.DatabasesArrayAsMap(new.Features.Oracle.Database.Databases)
	}

	newEnterpriseLicenseAlertThrown := false
	for _, newDb := range newDbs {

		var oldDb model.OracleDatabase
		if val, ok := previousDbs[newDb.Name]; ok {
			oldDb = val
		} else {
			oldDb = model.OracleDatabase{
				Licenses: []model.OracleDatabaseLicense{},
			}

			if err := hds.throwNewDatabaseAlert(newDb.Name, new.Hostname); err != nil {
				hds.Log.Error(err)
			}
		}

		if ((previous.Info.CPUCores < new.Info.CPUCores) ||
			(!model.HasEnterpriseLicense(oldDb) && model.HasEnterpriseLicense(newDb))) &&
			!newEnterpriseLicenseAlertThrown {
			if err := hds.throwNewEnterpriseLicenseAlert(new.Hostname); err != nil {
				hds.Log.Error(err)
			}
			newEnterpriseLicenseAlertThrown = true
		}

		//TODO notify Oracle STD

		diff := model.DiffLicenses(oldDb.Licenses, newDb.Licenses)

		activatedLicenses := []string{}
		for license, val := range diff {
			if val == model.DiffFeatureActivated && license != "Oracle ENT" && license != "Oracle STD" && license != "Oracle EXE" {
				activatedLicenses = append(activatedLicenses, license)
			}
		}

		//TODO Compare activated licenses with previous hostdata
		if len(activatedLicenses) > 0 {
			if err := hds.throwActivatedFeaturesAlert(newDb.Name, new.Hostname, activatedLicenses); err != nil {
				hds.Log.Error(err)
			}
		}
	}

	return nil
}
