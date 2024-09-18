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
	key        string
	references []string
}

type PortfolioReference struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Disabled bool   `json:"disabled"`
}

type PortfoliosResponse struct {
	Portfolios []Portfolio `json:"portfolios"`
}

type SubView struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type ShowPortfolio struct {
	Key      string    `json:"key"`
	SubViews []SubView `json:"subViews"`
}

func resourceSonarqubePortfolioHierarchy() *schema.Resource {
	return &schema.Resource{
		Create: resourceSonarqubePortfolioHierarchyCreate,
		Read:   resourceSonarqubePortfolioHierarchyRead,
		Update: resourceSonarqubePortfolioHierarchyUpdate,
		Delete: resourceSonarqubePortfolioHierarchyDelete,

		// Define the fields of this schema.
		Schema: map[string]*schema.Schema{
			"key": {
				Type:        schema.TypeString,
				Description: "Key of the portfolio.",
				Required:    true,
				Computed:    false,
			},
			"references": {
				Type:        schema.TypeList,
				Description: "List of portfolio keys to be added.",
				Required:    true,
				Computed:    false,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
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
	oldReferences, newReferences := d.GetChange("references")
	oldRef := processListString(oldReferences.([]interface{}))
	newRef := processListString(newReferences.([]interface{}))
	log.Printf("newReferences: %s", newRef)

	if d.HasChange("key") {
		oldKey, newKey := d.GetChange("key")
		err := GetPortfolioReference(d, m)
		if err != nil {
			log.Printf("newKey: %s", newKey)

			d.Set("key", oldKey.(string))
			return err
		}

		data := PortfolioHierarchy{oldKey.(string), oldRef}
		err = DeleteChildPortfolio(&data, d, m)
		if err != nil {
			return err
		}

		return resourceSonarqubePortfolioHierarchyCreate(d, m)
	}

	if d.HasChange("references") {
		err := GetPortfolioReference(d, m)
		if err != nil {
			d.Set("references", oldReferences)
			return err
		}
		oldRef := processListString(oldReferences.([]interface{}))
		newRef := processListString(newReferences.([]interface{}))

		addReferences, removeReferences := getDifferences(oldRef, newRef)

		data := PortfolioHierarchy{d.Get("key").(string), removeReferences}
		if len(removeReferences) > 0 {
			err := DeleteChildPortfolio(&data, d, m)
			if err != nil {
				return err
			}
		}

		if len(addReferences) > 0 {
			data.references = addReferences
			err := PostChildPortfolio(&data, d, m)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func resourceSonarqubePortfolioHierarchyRead(d *schema.ResourceData, m interface{}) error {

	log.Printf("Execute read method")
	portfolioHierarchyObject := GetPortfolioResourceInput(d, m)
	portfolioShow, err := GetPortfolioHierarchy(portfolioHierarchyObject, d, m)
	if err != nil {
		return err
	}

	reference := []string{}
	for _, ref := range portfolioShow.SubViews {
		reference = append(reference, ref.Key)
	}
	d.Set("key", portfolioShow.Key)
	d.Set("reference", reference)

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

func getDifferences(oldRefs, newRefs []string) (add, remove []string) {
	oldSet := make(map[string]struct{})
	newSet := make(map[string]struct{})

	for _, ref := range oldRefs {
		oldSet[ref] = struct{}{}
	}
	for _, ref := range newRefs {
		newSet[ref] = struct{}{}
	}

	for ref := range newSet {
		if _, exists := oldSet[ref]; !exists {
			add = append(add, ref)
		}
	}

	for ref := range oldSet {
		if _, exists := newSet[ref]; !exists {
			remove = append(remove, ref)
		}
	}
	return
}

func processListString(rawReferences []interface{}) []string {
	references := make([]string, len(rawReferences))

	for i, ref := range rawReferences {
		references[i] = ref.(string)
	}

	return references
}

func GetPortfolioResourceInput(d *schema.ResourceData, m interface{}) *PortfolioHierarchy {
	references := processListString(d.Get("references").([]interface{}))

	return &PortfolioHierarchy{
		key:        d.Get("key").(string),
		references: references,
	}
}

func GetPortfolioReference(d *schema.ResourceData, m interface{}) error {
	portfolioHierarchyObject := GetPortfolioResourceInput(d, m)

	apiURL := "api/views/portfolios"
	method := "GET"
	rawQuery := url.Values{
		"portfolio": []string{portfolioHierarchyObject.key},
	}.Encode()

	resp, err := executeHttpMethod(method, m, apiURL, rawQuery, http.StatusOK)
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

	removeOwnPortfolio(portfolioHierarchyObject.key, &listKeyRefs)

	err = validateReference(portfolioHierarchyObject.references, listKeyRefs)
	if err != nil {
		return err
	}

	return nil
}

func GetPortfolioHierarchy(data *PortfolioHierarchy, d *schema.ResourceData, m interface{}) (*ShowPortfolio, error) {
	apiURL := "api/views/show"
	method := "GET"
	rawQuery := url.Values{
		"key": []string{data.key},
	}.Encode()

	resp, err := executeHttpMethod(method, m, apiURL, rawQuery, http.StatusOK)
	if err != nil {
		return nil, err
	}

	portfolioShow := ShowPortfolio{}
	err = json.NewDecoder(resp.Body).Decode(&portfolioShow)

	return &portfolioShow, nil
}

func PostChildPortfolio(data *PortfolioHierarchy, d *schema.ResourceData, m interface{}) error {
	apiURL := "api/views/add_portfolio"
	method := "POST"
	for _, ref := range data.references {
		rawQuery := url.Values{
			"portfolio": []string{data.key},
			"reference": []string{ref},
		}.Encode()

		resp, err := executeHttpMethod(method, m, apiURL, rawQuery, http.StatusOK)
		if err != nil {
			return err
		}
		log.Printf("Response: %+v", resp)
	}

	return nil
}

func DeleteChildPortfolio(data *PortfolioHierarchy, d *schema.ResourceData, m interface{}) error {
	apiURL := "api/views/remove_portfolio"
	method := "POST"
	for _, ref := range data.references {
		rawQuery := url.Values{
			"portfolio": []string{data.key},
			"reference": []string{ref},
		}.Encode()

		resp, err := executeHttpMethod(method, m, apiURL, rawQuery, http.StatusNoContent)
		if err != nil {
			return err
		}
		log.Printf("Response: %+v", resp)

	}

	return nil
}

func validateReference(inputRef []string, unSelectedRef []string) error {
	refMap := make(map[string]bool)
	for _, ref := range unSelectedRef {
		refMap[ref] = true
	}
	// Kiểm tra từng phần tử trong inputRef
	for _, ref := range inputRef {
		if !refMap[ref] {
			// Nếu phần tử không tồn tại trong unSelectedRef, trả về lỗi
			return fmt.Errorf("reference %s is not exits in Unselected Reference", ref)
		}
	}
	return nil
}

func removeOwnPortfolio(parent string, unSelectedRef *[]string) {
	ref := *unSelectedRef

	var result []string
	for _, item := range ref {
		if item != parent {
			result = append(result, item)
		}
	}
	*unSelectedRef = result
}

func executeHttpMethod(method string, m interface{}, apiURL string, rawQuery string, status int) (*http.Response, error) {
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
