// Change category_description to categories


package main

import (
	"bufio"
	"os"
	"fmt"
	"net/http"
	"github.com/PuerkitoBio/goquery"
	"strings"
	"regexp"
	"strconv"
)

const (
	CATEGORY_LINK_SELECTOR = ".arrow a"
)

var (
	category_descriptions map[int]string = make(map[int]string)
)

type category struct {
	description string
	index int
}

func pull_satellite_information(nssdc_url string) (satellite_description string, error error) {
	response, error := http.Get(nssdc_url)

	if error == nil {
		defer response.Body.Close()
		document, _ := goquery.NewDocumentFromReader(response.Body)
		
		var paragraphs []string
		
		document.Find(".urone p").Each(func(i int, selection *goquery.Selection) {
			text := selection.Text()
			if len(strings.TrimSpace(text)) != 0 {
				paragraphs = append(paragraphs, text)
			}
		})
		satellite_description = strings.Join(paragraphs, "\n")
	} else {
		satellite_description = ""
	}

	return satellite_description, error
}

func pull_category_information(ny20_category_url string) (category_description string, error error) {
	response, error := http.Get("https://www.n2yo.com" + ny20_category_url)
	
	if error == nil {
		defer response.Body.Close()
		document, _ := goquery.NewDocumentFromReader(response.Body)
		the_table := document.Find("table").Text()
		title := document.Find("h1").Text()
		re := regexp.MustCompile("(.*\n)*" + title + "\\s*(?P<description>.*\n)(.*\n)*.*")
		category_description = re.ReplaceAllString(the_table, "${description}")
	} else {
		category_description = ""
	}

	return category_description, error
}

func pull(norad_id int) (local_category_descriptions []category, satellite_description string, error error) {
	
	response, error := http.Get(fmt.Sprintf("https://www.n2yo.com/satellite/?s=%d#results", norad_id))
	
	if error == nil {
		defer response.Body.Close()
		document, _ := goquery.NewDocumentFromReader(response.Body)
		re := regexp.MustCompile("[[:digit:]]*$")
		
		document.Find(".arrow a").Each(
			func(i int, selection *goquery.Selection) {
				ny20_category_url, _ := selection.Attr("href")
				number_at_end, _ := strconv.Atoi(re.FindString(ny20_category_url))
				_, exists := category_descriptions[number_at_end]

				var category_description string
				
				if ! exists { 
					category_description, _ = pull_category_information(ny20_category_url)
					category_descriptions[number_at_end] = category_description
				} else {
					category_description = category_descriptions[number_at_end]
				}
				
				local_category_descriptions = append(local_category_descriptions, category {description:category_description, index:number_at_end})
			})
		
		document.Find("tbody a").Each(
			func(i int, selection *goquery.Selection) {
				nssdc_url, _ := selection.Attr("href")
				if strings.Contains(nssdc_url, "nssdc.gsfc.nasa.gov") {
					satellite_description, _ = pull_satellite_information(nssdc_url)
				}
			})
	}

	return
}

func main() {

	const (
		output_dir = "descriptions/"
		satellite_descriptions_dir = output_dir + "satellites/"
		category_descriptions_dir = output_dir + "categories/"
		images_dir = output_dir + "images/"
	)

	os.Mkdir(output_dir, 0755)
	os.Mkdir(satellite_descriptions_dir, 0755)
	os.Mkdir(category_descriptions_dir, 0755)

	type satellite struct {
		norad_id int
		has_description bool
		categories []int
	}

	var satellites []satellite
	
	for i:=0; i<53000; i++ {
		categories, satellite_description, _ := pull(i)

		satellite := satellite {norad_id: i}

		if len(satellite_description) != 0 {
			path := satellite_descriptions_dir + strconv.Itoa(i)
			_, err := os.Stat(path)
			if err != nil {
				data := []byte(satellite_description)
				os.WriteFile(path, data, 0644)
			}

			satellite.has_description = true
		} else {
			satellite.has_description = false
		}
		
		for _, category := range categories {
			path := category_descriptions_dir + strconv.Itoa(category.index)
			_, err := os.Stat(path)
			if err != nil {
				data := []byte(category.description)
				os.WriteFile(path, data, 0644)
			}
			satellite.categories = append(satellite.categories, category.index)
		}

		satellites = append(satellites, satellite)
	}

	
	f, _ := os.Create(output_dir + "satellites.tsv")
	defer f.Close()
	w := bufio.NewWriter(f)
	
	for _, satellite := range satellites {
		// Gotards be like: Why would you need the ternary operator?
		flag := ""
		if satellite.has_description {
			flag = "?"
		}

		categories := ""
		for i, category := range satellite.categories {
			if i != len(satellite.categories) - 1 {
				categories += strconv.Itoa(category) + ","
			} else {
				categories += strconv.Itoa(category)
			}
		}

		w.WriteString(fmt.Sprintf("%d\t%s\t%s", satellite.norad_id, flag, categories))
	}
	
	w.Flush()
}
