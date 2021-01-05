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

package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/360EntSecGroup-Skylar/excelize"
	"github.com/ercole-io/ercole/v2/api-service/dto"
	"github.com/ercole-io/ercole/v2/utils"
	"github.com/golang/gddo/httputil"
	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// SearchHosts search hosts data using the filters in the request
func (ctrl *APIController) SearchHosts(w http.ResponseWriter, r *http.Request) {
	filters, err := getSearchHostFilters(r)
	if err != nil {
		utils.WriteAndLogError(ctrl.Log, w, http.StatusUnprocessableEntity, err)
		return
	}

	requestContentType := httputil.NegotiateContentType(r,
		[]string{
			"application/json",
			"application/vnd.oracle.lms+vnd.ms-excel.sheet.macroEnabled.12",
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			"application/vnd.ercole.mongohostdata+json",
		},
		"application/json")

	switch requestContentType {
	case "application/json":
		ctrl.searchHostsJSON(w, r, filters)
	case "application/vnd.oracle.lms+vnd.ms-excel.sheet.macroEnabled.12":
		ctrl.searchHostsLMS(w, r, filters)
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		ctrl.searchHostsXLSX(w, r, filters)
	}
}

// searchHostsJSON search hosts data using the filters in the request returning it in JSON
func (ctrl *APIController) searchHostsJSON(w http.ResponseWriter, r *http.Request, filters *dto.SearchHostsFilters) {
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "full"
	} else if mode != "full" && mode != "summary" && mode != "lms" && mode != "mhd" && mode != "hostnames" {
		utils.WriteAndLogError(ctrl.Log, w, http.StatusUnprocessableEntity, utils.NewAdvancedErrorPtr(errors.New("Invalid mode value"), http.StatusText(http.StatusUnprocessableEntity)))
		return
	}

	hosts, err := ctrl.Service.SearchHosts(mode, *filters)
	if err != nil {
		utils.WriteAndLogError(ctrl.Log, w, http.StatusInternalServerError, err)
		return
	}

	if mode == "hostnames" {
		hostnames := make([]string, len(hosts))
		for i, h := range hosts {
			hostnames[i] = h["hostname"].(string)
		}
		utils.WriteJSONResponse(w, http.StatusOK, hostnames)
	} else {
		if filters.PageNumber == -1 || filters.PageSize == -1 {
			utils.WriteJSONResponse(w, http.StatusOK, hosts)
		} else {
			utils.WriteJSONResponse(w, http.StatusOK, hosts[0])
		}
	}
}

// searchHostsLMS search hosts data using the filters in the request returning it in LMS+XLSX
func (ctrl *APIController) searchHostsLMS(w http.ResponseWriter, r *http.Request, filters *dto.SearchHostsFilters) {
	filters.PageNumber, filters.PageSize = -1, -1
	lms, aerr := ctrl.Service.SearchHostsAsLMS(*filters)
	if aerr != nil {
		utils.WriteAndLogError(ctrl.Log, w, http.StatusInternalServerError, aerr)
		return
	}

	utils.WriteXLSMResponse(w, lms)
}

// searchHostsXLSX search hosts data using the filters in the request returning it in XLSX
//TODO Move in service
func (ctrl *APIController) searchHostsXLSX(w http.ResponseWriter, r *http.Request, filters *dto.SearchHostsFilters) {
	filters.PageNumber, filters.PageSize = -1, -1
	hosts, aerr := ctrl.Service.SearchHosts("summary", *filters)
	if aerr != nil {
		utils.WriteAndLogError(ctrl.Log, w, http.StatusInternalServerError, aerr)
		return
	}

	sheets, err := excelize.OpenFile(ctrl.Config.ResourceFilePath + "/templates/template_hosts.xlsx")
	if err != nil {
		utils.WriteAndLogError(ctrl.Log, w, http.StatusInternalServerError, utils.NewAdvancedErrorPtr(err, "READ_TEMPLATE"))
		return
	}

	for i, val := range hosts {
		sheets.SetCellValue("Hosts", fmt.Sprintf("A%d", i+2), val["hostname"])
		sheets.SetCellValue("Hosts", fmt.Sprintf("B%d", i+2), val["environment"])
		if val["cluster"] != nil && val["virtualizationNode"] != nil {
			sheets.SetCellValue("Hosts", fmt.Sprintf("D%d", i+2), val["cluster"])
			sheets.SetCellValue("Hosts", fmt.Sprintf("E%d", i+2), val["virtualizationNode"])
		}
		sheets.SetCellValue("Hosts", fmt.Sprintf("F%d", i+2), val["agentVersion"])
		sheets.SetCellValue("Hosts", fmt.Sprintf("G%d", i+2), val["createdAt"].(primitive.DateTime).Time().UTC().String())
		sheets.SetCellValue("Hosts", fmt.Sprintf("I%d", i+2), val["os"])
		sheets.SetCellValue("Hosts", fmt.Sprintf("J%d", i+2), val["kernel"])
		sheets.SetCellValue("Hosts", fmt.Sprintf("K%d", i+2), val["oracleClusterware"])
		sheets.SetCellValue("Hosts", fmt.Sprintf("L%d", i+2), val["sunCluster"])
		sheets.SetCellValue("Hosts", fmt.Sprintf("M%d", i+2), val["veritasClusterServer"])
		sheets.SetCellValue("Hosts", fmt.Sprintf("N%d", i+2), val["hardwareAbstraction"])
		sheets.SetCellValue("Hosts", fmt.Sprintf("O%d", i+2), val["hardwareAbstractionTechnology"])
		sheets.SetCellValue("Hosts", fmt.Sprintf("P%d", i+2), val["cpuThreads"])
		sheets.SetCellValue("Hosts", fmt.Sprintf("Q%d", i+2), val["cpuCores"])
		sheets.SetCellValue("Hosts", fmt.Sprintf("R%d", i+2), val["cpuSockets"])
		sheets.SetCellValue("Hosts", fmt.Sprintf("S%d", i+2), val["memTotal"])
		sheets.SetCellValue("Hosts", fmt.Sprintf("T%d", i+2), val["swapTotal"])
		sheets.SetCellValue("Hosts", fmt.Sprintf("U%d", i+2), val["cpuModel"])
	}

	utils.WriteXLSXResponse(w, sheets)
}

func getSearchHostFilters(r *http.Request) (*dto.SearchHostsFilters, utils.AdvancedErrorInterface) {
	f := dto.SearchHostsFilters{}
	var aerr utils.AdvancedErrorInterface

	f.Search = strings.Split(r.URL.Query().Get("search"), " ")

	f.SortBy = r.URL.Query().Get("sort-by")

	if f.SortDesc, aerr = utils.Str2bool(r.URL.Query().Get("sort-desc"), false); aerr != nil {
		return nil, aerr
	}

	f.Location = r.URL.Query().Get("location")
	f.Environment = r.URL.Query().Get("environment")

	if f.OlderThan, aerr = utils.Str2time(r.URL.Query().Get("older-than"), utils.MAX_TIME); aerr != nil {
		return nil, aerr
	}

	if f.PageNumber, aerr = utils.Str2int(r.URL.Query().Get("page"), -1); aerr != nil {
		return nil, aerr
	}

	if f.PageSize, aerr = utils.Str2int(r.URL.Query().Get("size"), -1); aerr != nil {
		return nil, aerr
	}

	f.Hostname = r.URL.Query().Get("hostname")
	f.Database = r.URL.Query().Get("database")
	f.Technology = r.URL.Query().Get("technology")
	f.HardwareAbstractionTechnology = r.URL.Query().Get("hardware-abstraction-technology")
	if r.URL.Query().Get("cluster") == "NULL" {
		f.Cluster = nil
	} else {
		f.Cluster = new(string)
		*f.Cluster = r.URL.Query().Get("cluster")
	}
	f.VirtualizationNode = r.URL.Query().Get("virtualization-node")
	f.OperatingSystem = r.URL.Query().Get("operating-system")
	f.Kernel = r.URL.Query().Get("kernel")
	if f.LTEMemoryTotal, aerr = utils.Str2float64(r.URL.Query().Get("memory-total-lte"), -1); aerr != nil {
		return nil, aerr
	}
	if f.GTEMemoryTotal, aerr = utils.Str2float64(r.URL.Query().Get("memory-total-gte"), -1); aerr != nil {
		return nil, aerr
	}
	if f.LTESwapTotal, aerr = utils.Str2float64(r.URL.Query().Get("swap-total-lte"), -1); aerr != nil {
		return nil, aerr
	}
	if f.GTESwapTotal, aerr = utils.Str2float64(r.URL.Query().Get("swap-total-gte"), -1); aerr != nil {
		return nil, aerr
	}
	if r.URL.Query().Get("is-member-of-cluster") == "" {
		f.IsMemberOfCluster = nil
	} else {
		f.IsMemberOfCluster = new(bool)
		if *f.IsMemberOfCluster, aerr = utils.Str2bool(r.URL.Query().Get("is-member-of-cluster"), false); aerr != nil {
			return nil, aerr
		}
	}
	f.CPUModel = r.URL.Query().Get("cpu-model")
	if f.LTECPUCores, aerr = utils.Str2int(r.URL.Query().Get("cpu-cores-lte"), -1); aerr != nil {
		return nil, aerr
	}
	if f.GTECPUCores, aerr = utils.Str2int(r.URL.Query().Get("cpu-cores-gte"), -1); aerr != nil {
		return nil, aerr
	}
	if f.LTECPUThreads, aerr = utils.Str2int(r.URL.Query().Get("cpu-threads-lte"), -1); aerr != nil {
		return nil, aerr
	}
	if f.GTECPUThreads, aerr = utils.Str2int(r.URL.Query().Get("cpu-threads-gte"), -1); aerr != nil {
		return nil, aerr
	}

	return &f, nil
}

// GetHost return all'informations about the host requested in the id path variable
func (ctrl *APIController) GetHost(w http.ResponseWriter, r *http.Request) {
	choiche := httputil.NegotiateContentType(r, []string{"application/json", "application/vnd.ercole.mongohostdata+json"}, "application/json")

	switch choiche {
	case "application/json":
		ctrl.GetHostJSON(w, r)
	case "application/vnd.ercole.mongohostdata+json":
		ctrl.GetHostMongoJSON(w, r)
	}
}

// GetHostJSON return all'informations about the host requested in the id path variable
func (ctrl *APIController) GetHostJSON(w http.ResponseWriter, r *http.Request) {
	var olderThan time.Time
	var err utils.AdvancedErrorInterface

	hostname := mux.Vars(r)["hostname"]

	if olderThan, err = utils.Str2time(r.URL.Query().Get("older-than"), utils.MAX_TIME); err != nil {
		utils.WriteAndLogError(ctrl.Log, w, http.StatusUnprocessableEntity, err)
		return
	}

	host, err := ctrl.Service.GetHost(hostname, olderThan, false)
	if err == utils.AerrHostNotFound {
		utils.WriteAndLogError(ctrl.Log, w, http.StatusNotFound, err)
		return
	} else if err != nil {
		utils.WriteAndLogError(ctrl.Log, w, http.StatusInternalServerError, err)
		return
	}

	utils.WriteJSONResponse(w, http.StatusOK, host)
}

// GetHostMongoJSON return all'informations about the host requested in the id path variable
func (ctrl *APIController) GetHostMongoJSON(w http.ResponseWriter, r *http.Request) {
	var olderThan time.Time
	var aerr utils.AdvancedErrorInterface

	hostname := mux.Vars(r)["hostname"]

	if olderThan, aerr = utils.Str2time(r.URL.Query().Get("older-than"), utils.MAX_TIME); aerr != nil {
		utils.WriteAndLogError(ctrl.Log, w, http.StatusUnprocessableEntity, aerr)
		return
	}

	host, aerr := ctrl.Service.GetHost(hostname, olderThan, true)
	if aerr == utils.AerrHostNotFound {
		utils.WriteAndLogError(ctrl.Log, w, http.StatusNotFound, aerr)
		return
	} else if aerr != nil {
		utils.WriteAndLogError(ctrl.Log, w, http.StatusInternalServerError, aerr)
		return
	}

	utils.WriteExtJSONResponse(ctrl.Log, w, http.StatusOK, host)
}

// ListLocations list locations using the filters in the request
func (ctrl *APIController) ListLocations(w http.ResponseWriter, r *http.Request) {
	var location string
	var environment string
	var olderThan time.Time

	var err utils.AdvancedErrorInterface
	location = r.URL.Query().Get("location")
	environment = r.URL.Query().Get("environment")

	if olderThan, err = utils.Str2time(r.URL.Query().Get("older-than"), utils.MAX_TIME); err != nil {
		utils.WriteAndLogError(ctrl.Log, w, http.StatusUnprocessableEntity, err)
		return
	}

	locations, err := ctrl.Service.ListLocations(location, environment, olderThan)
	if err != nil {
		utils.WriteAndLogError(ctrl.Log, w, http.StatusInternalServerError, err)
		return
	}

	utils.WriteJSONResponse(w, http.StatusOK, locations)
}

// ListEnvironments list the environments using the filters in the request
func (ctrl *APIController) ListEnvironments(w http.ResponseWriter, r *http.Request) {
	var location string
	var environment string
	var olderThan time.Time

	var err utils.AdvancedErrorInterface
	location = r.URL.Query().Get("location")
	environment = r.URL.Query().Get("environment")

	if olderThan, err = utils.Str2time(r.URL.Query().Get("older-than"), utils.MAX_TIME); err != nil {
		utils.WriteAndLogError(ctrl.Log, w, http.StatusUnprocessableEntity, err)
		return
	}

	environments, err := ctrl.Service.ListEnvironments(location, environment, olderThan)
	if err != nil {
		utils.WriteAndLogError(ctrl.Log, w, http.StatusInternalServerError, err)
		return
	}

	utils.WriteJSONResponse(w, http.StatusOK, environments)
}

// ArchiveHost archive the specified host in the request
func (ctrl *APIController) ArchiveHost(w http.ResponseWriter, r *http.Request) {
	if ctrl.Config.APIService.ReadOnly {
		utils.WriteAndLogError(ctrl.Log, w, http.StatusForbidden, utils.NewAdvancedErrorPtr(errors.New("The API is disabled because the service is put in read-only mode"), "FORBIDDEN_REQUEST"))
		return
	}

	hostname := mux.Vars(r)["hostname"]

	aerr := ctrl.Service.ArchiveHost(hostname)
	if aerr == utils.AerrHostNotFound {
		utils.WriteAndLogError(ctrl.Log, w, http.StatusNotFound, aerr)
	} else if aerr != nil {
		utils.WriteAndLogError(ctrl.Log, w, http.StatusInternalServerError, aerr)
		return
	}

	utils.WriteJSONResponse(w, http.StatusOK, nil)
}
