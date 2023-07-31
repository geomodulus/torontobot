package opendata

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type APIResponse struct {
	Help    string `json:"help"`
	Success bool   `json:"success"`
	Result  Result `json:"result"`
}

type Result struct {
	Author                 string         `json:"author"`
	AuthorEmail            string         `json:"author_email"`
	CollectionMethod       string         `json:"collection_method"`
	CreatorUserID          string         `json:"creator_user_id"`
	DatasetCategory        string         `json:"dataset_category"`
	DatePublished          CustomTime     `json:"date_published"`
	Excerpt                string         `json:"excerpt"`
	Formats                string         `json:"formats"`
	ID                     string         `json:"id"`
	InformationURL         string         `json:"information_url"`
	IsRetired              string         `json:"is_retired"`
	IsOpen                 bool           `json:"isopen"`
	LastRefreshed          CustomTime     `json:"last_refreshed"`
	LicenseID              string         `json:"license_id"`
	LicenseTitle           string         `json:"license_title"`
	Limitations            string         `json:"limitations"`
	Maintainer             string         `json:"maintainer"`
	MaintainerEmail        string         `json:"maintainer_email"`
	MetadataCreated        CustomTime     `json:"metadata_created"`
	MetadataModified       CustomTime     `json:"metadata_modified"`
	Name                   string         `json:"name"`
	Notes                  string         `json:"notes"`
	NumResources           int            `json:"num_resources"`
	NumTags                int            `json:"num_tags"`
	Organization           Organization   `json:"organization"`
	OwnerDivision          string         `json:"owner_division"`
	OwnerEmail             string         `json:"owner_email"`
	OwnerOrg               string         `json:"owner_org"`
	Private                bool           `json:"private"`
	RefreshRate            string         `json:"refresh_rate"`
	State                  string         `json:"state"`
	Title                  string         `json:"title"`
	Topics                 string         `json:"topics"`
	Type                   string         `json:"type"`
	Version                string         `json:"version"`
	Resources              []Resource     `json:"resources"`
	Tags                   []Tag          `json:"tags"`
	Groups                 []Group        `json:"groups"`
	RelationshipsAsSubject []Relationship `json:"relationships_as_subject"`
	RelationshipsAsObject  []Relationship `json:"relationships_as_object"`
}

type Organization struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Title          string     `json:"title"`
	Type           string     `json:"type"`
	Description    string     `json:"description"`
	ImageURL       string     `json:"image_url"`
	Created        CustomTime `json:"created"`
	IsOrganization bool       `json:"is_organization"`
	ApprovalStatus string     `json:"approval_status"`
	State          string     `json:"state"`
}

type Resource struct {
	CacheLastUpdated     interface{} `json:"cache_last_updated"`
	CacheURL             interface{} `json:"cache_url"`
	Created              CustomTime  `json:"created"`
	DatastoreActive      bool        `json:"datastore_active"`
	Format               string      `json:"format"`
	Hash                 string      `json:"hash"`
	ID                   string      `json:"id"`
	IsDatastoreCacheFile bool        `json:"is_datastore_cache_file"`
	IsPreview            BoolString  `json:"is_preview"`
	LastModified         CustomTime  `json:"last_modified"`
	MetadataModified     CustomTime  `json:"metadata_modified"`
	MimeType             string      `json:"mimetype"`
	MimeTypeInner        interface{} `json:"mimetype_inner"`
	Name                 string      `json:"name"`
	PackageID            string      `json:"package_id"`
	Position             int         `json:"position"`
	ResourceType         interface{} `json:"resource_type"`
	RevisionID           string      `json:"revision_id"`
	Size                 int         `json:"size"`
	State                string      `json:"state"`
	URL                  string      `json:"url"`
	URLType              string      `json:"url_type"`
}

type Tag struct {
	DisplayName  string      `json:"display_name"`
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	State        string      `json:"state"`
	VocabularyID interface{} `json:"vocabulary_id"`
}

type Group struct {
	// this struct can be filled based on the data returned in the "groups" field
}

type Relationship struct {
	// this struct can be filled based on the data returned in the "relationships_as_subject" and "relationships_as_object" fields
}

type BoolString bool

func (bs *BoolString) UnmarshalJSON(data []byte) error {
	asString := strings.Trim(string(data), "\"") // remove quotes if they exist

	if asString == "True" || asString == "true" {
		*bs = BoolString(true)
		return nil
	} else if asString == "False" || asString == "false" {
		*bs = BoolString(false)
		return nil
	}

	// handle actual bool type
	asBool, err := strconv.ParseBool(asString)
	if err != nil {
		return err
	}

	*bs = BoolString(asBool)
	return nil
}

// CustomTime is a custom time.Time type for our special unmarshal case
type CustomTime time.Time

func (ct *CustomTime) UnmarshalJSON(b []byte) (err error) {
	if b == nil || string(b) == "null" {
		// no date present in the JSON
		return nil
	}

	s := strings.Trim(string(b), "\"")
	nt, err := time.Parse("2006-01-02 15:04:05.999999", s)
	if err != nil {
		// if the first format fails, try the second one
		nt, err = time.Parse("2006-01-02T15:04:05.999999", s)

	}
	*ct = CustomTime(nt)
	return
}

func Get(id string) (*APIResponse, error) {
	resp, err := http.Get("https://ckan0.cf.opendata.inter.prod-toronto.ca/api/3/action/package_show?id=" + id)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apiResponse APIResponse
	if err = json.Unmarshal(body, &apiResponse); err != nil {
		return nil, err
	}
	return &apiResponse, nil
}
