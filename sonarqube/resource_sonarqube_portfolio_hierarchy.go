package sonarqube

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

type PortfolioHierarchy struct {
	key       string
	reference string
}

type PortfolioReference struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Disabled bool   `json:"disabled"`
}

type PortfoliosResponse struct {
	Portfolios []Portfolio `json:"portfolios"`
}

func resourceSonarqubePortfolioHierarchy() *schema.Resource {
	return &schema.Resource{
		Create: resourceSonarqubePortfolioHierarchyCreate,
		Read:   resourceSonarqubePortfolioHierarchyRead,
		Update: resourceSonarqubePortfolioHierarchyUpdate,
		Delete: resourceSonarqubePortfolioHierarchyDelete,
		Importer: &schema.ResourceImporter{
			State: resourceSonarqubeGroupImport,
		},

		// Define the fields of this schema.
		Schema: map[string]*schema.Schema{
			"key": {
				Type:        schema.TypeString,
				Description: "Key of the portfolio.",
				Required:    true,
				Computed:    false,
			},
			"reference": {
				Type:        schema.TypeString,
				Description: "Key of the portfolio to be added.",
				Required:    true,
				Computed:    false,
			},
		},
	}
}

func resourceSonarqubePortfolioHierarchyCreate(d *schema.ResourceData, m interface{}) error {

	log.Printf("Execute create method")

	err := GetPortfolioReference(d, m)
	if err != nil {
		return err
	}

	portfolioHierarchyObject := GetPortfolioResourceInput(d, m)
	err = PostChildPortfolio(portfolioHierarchyObject, d, m)
	if err != nil {
		return err
	}

	id := fmt.Sprintf("%v-%v", d.Get("key").(string), "parent")
	d.SetId(id)

	return resourceSonarqubePortfolioHierarchyRead(d, m)
}

func resourceSonarqubePortfolioHierarchyUpdate(d *schema.ResourceData, m interface{}) error {

	log.Printf("Execute update method")

	if d.HasChange("key") {
		err := GetPortfolioReference(d, m)
		if err != nil {
			return err
		}

		oldValue, newValue := d.GetChange("reference")
		if oldValue != newValue {
			portfolioHierarchyObject := GetPortfolioResourceInput(d, m)
			portfolioHierarchyObject.reference = oldValue.(string)

			err := DeleteChildPortfolio(portfolioHierarchyObject, d, m)
			if err != nil {
				return err
			}
		}

		return resourceSonarqubePortfolioHierarchyCreate(d, m)
	}

	if d.HasChange("reference") {
		err := GetPortfolioReference(d, m)
		if err != nil {
			return err
		}

		oldValue, newValue := d.GetChange("reference")
		if oldValue != newValue {
			portfolioHierarchyObject := GetPortfolioResourceInput(d, m)
			portfolioHierarchyObject.reference = oldValue.(string)
			err := DeleteChildPortfolio(portfolioHierarchyObject, d, m)
			if err != nil {
				return err
			}

			err = PostChildPortfolio(portfolioHierarchyObject, d, m)
			if err != nil {
				return err
			}
		}

	}

	return nil
}

func resourceSonarqubePortfolioHierarchyRead(d *schema.ResourceData, m interface{}) error {

	log.Printf("Execute read method")

	return nil
}

func resourceSonarqubePortfolioHierarchyDelete(d *schema.ResourceData, m interface{}) error {

	log.Printf("Execute delete method")

	portfolioHierarchyObject := GetPortfolioResourceInput(d, m)
	err := DeleteChildPortfolio(portfolioHierarchyObject, d, m)
	if err != nil {
		return err
	}
	d.SetId("")

	return nil
}

func GetPortfolioResourceInput(d *schema.ResourceData, m interface{}) *PortfolioHierarchy {
	return &PortfolioHierarchy{
		key:       d.Get("key").(string),
		reference: d.Get("reference").(string),
	}
}

func GetPortfolioReference(d *schema.ResourceData, m interface{}) error {
	portfolioHierarchyObject := GetPortfolioResourceInput(d, m)

	apiURL := "api/views/portfolios"
	method := "GET"
	rawQuery := url.Values{
		"portfolio": []string{portfolioHierarchyObject.key},
	}.Encode()

	resp, err := executeHttpMethod(method, d, m, apiURL, rawQuery, http.StatusOK)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	portfolioReferenceResponse := PortfoliosResponse{}
	err = json.NewDecoder(resp.Body).Decode(&portfolioReferenceResponse)
	if err != nil {
		log.Printf("fail to decode into struct: %+v", err)
		return err
	}

	var listKeyRefs []string
	for _, portfolio := range portfolioReferenceResponse.Portfolios {
		listKeyRefs = append(listKeyRefs, portfolio.Key)
	}

	removeOwnPortlifo(portfolioHierarchyObject.key, &listKeyRefs)

	err = validateReference(portfolioHierarchyObject.reference, listKeyRefs)
	if err != nil {
		return err
	}

	return nil
}

func GetPortfolioHierarchy(data *PortfolioHierarchy, d *schema.ResourceData, m interface{}) ([]string, error) {
	apiURL := "api/views/show"
	method := "GET"
	rawQuery := url.Values{
		"portfolio": []string{data.key},
	}.Encode()

	resp, err := executeHttpMethod(method, d, m, apiURL, rawQuery, http.StatusOK)
	if err != nil {
		return nil, err
	}
	log.Printf("Response: %+v", resp)

	return nil, nil
}

func PostChildPortfolio(data *PortfolioHierarchy, d *schema.ResourceData, m interface{}) error {
	apiURL := "api/views/add_portfolio"
	method := "POST"
	rawQuery := url.Values{
		"portfolio": []string{data.key},
		"reference": []string{data.reference},
	}.Encode()

	resp, err := executeHttpMethod(method, d, m, apiURL, rawQuery, http.StatusOK)
	if err != nil {
		return err
	}
	log.Printf("Response: %+v", resp)

	return nil
}

func DeleteChildPortfolio(data *PortfolioHierarchy, d *schema.ResourceData, m interface{}) error {
	apiURL := "api/views/remove_portfolio"
	method := "POST"
	rawQuery := url.Values{
		"portfolio": []string{data.key},
		"reference": []string{data.reference},
	}.Encode()

	resp, err := executeHttpMethod(method, d, m, apiURL, rawQuery, http.StatusNoContent)
	if err != nil {
		return err
	}
	log.Printf("Response: %+v", resp)

	return nil
}

func validateReference(inputRef string, unSelectedRef []string) error {
	for _, ref := range unSelectedRef {
		if ref == inputRef {
			return nil
		}
	}
	return fmt.Errorf("Value '%s' in inputRef not exits in Unselected Reference", inputRef)
}

func removeOwnPortlifo(parent string, unSelectedRef *[]string) {
	ref := *unSelectedRef

	var result []string
	for _, item := range ref {
		if item != parent {
			result = append(result, item)
		}
	}
	*unSelectedRef = result
}

func executeHttpMethod(method string, d *schema.ResourceData, m interface{}, apiURL string, rawQuery string, status int) (*http.Response, error) {
	sonarQubeURL := m.(*ProviderConfiguration).sonarQubeURL
	sonarQubeURL.Path = strings.TrimSuffix(sonarQubeURL.Path, "/") + apiURL

	sonarQubeURL.RawQuery = rawQuery
	resp, err := httpRequestHelper(
		m.(*ProviderConfiguration).httpClient,
		method,
		sonarQubeURL.String(),
		status,
		"resourceSonarqubePortfolioHierarchy"+" - "+apiURL,
	)
	if err != nil {
		return nil, fmt.Errorf("Fail to make '%s' request '%s': '%v'", method, sonarQubeURL.Path, err)
	}

	return &resp, nil
}

// GET api/views/list List root portfolios.
// GET api/views/portfolios List portfolios that can be referenced.
// POST api/views/remove_portfolio Remove a reference to a portfolio.
// POST api/views/add_portfolio Add an existing portfolio to the structure of another portfolio.
// GET api/views/show Show the details of a portfolio, including its hierarchy and project selection mode.
