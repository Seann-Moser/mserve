package mserve

import (
	"encoding/json"
	"errors"
	"net/http"
)

type IPLookup func() (string, error)

var _ IPLookup = IpApiLookup
var _ IPLookup = ApiIPQueryLookup

type IPApi struct {
	Status      string  `json:"status"`
	Country     string  `json:"country"`
	CountryCode string  `json:"countryCode"`
	Region      string  `json:"region"`
	RegionName  string  `json:"regionName"`
	City        string  `json:"city"`
	Zip         string  `json:"zip"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	Timezone    string  `json:"timezone"`
	Isp         string  `json:"isp"`
	Org         string  `json:"org"`
	As          string  `json:"as"`
	Query       string  `json:"query"`
}

func IpApiLookup() (string, error) {
	resp, err := http.Get("https://ip-api.com/json/")
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", errors.New(resp.Status)
	}
	v := IPApi{}
	err = json.NewDecoder(resp.Body).Decode(&v)
	if err != nil {
		return "", err
	}
	return v.Query, nil
}

type ApiIpQuery struct {
	Ip  string `json:"ip"`
	Isp struct {
		Asn string `json:"asn"`
		Org string `json:"org"`
		Isp string `json:"isp"`
	} `json:"isp"`
	Location struct {
		Country     string  `json:"country"`
		CountryCode string  `json:"country_code"`
		City        string  `json:"city"`
		State       string  `json:"state"`
		Zipcode     string  `json:"zipcode"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		Timezone    string  `json:"timezone"`
		Localtime   string  `json:"localtime"`
	} `json:"location"`
	Risk struct {
		IsMobile     bool `json:"is_mobile"`
		IsVpn        bool `json:"is_vpn"`
		IsTor        bool `json:"is_tor"`
		IsProxy      bool `json:"is_proxy"`
		IsDatacenter bool `json:"is_datacenter"`
		RiskScore    int  `json:"risk_score"`
	} `json:"risk"`
}

func ApiIPQueryLookup() (string, error) {
	resp, err := http.Get("https://api.ipquery.io/?format=json")
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", errors.New(resp.Status)
	}
	v := ApiIpQuery{}
	err = json.NewDecoder(resp.Body).Decode(&v)
	if err != nil {
		return "", err
	}
	return v.Ip, nil
}
