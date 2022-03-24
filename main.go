// Change categoryDescription to categories


package main

import (
	"flag"
	"time"
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
	outputDir = "descriptions/"
	satelliteDescriptionsDir = outputDir + "satellites/"
	categoryDescriptionsDir = outputDir + "categories/"
	imagesDir = outputDir + "images/"
	images_dir = outputDir + "images/"
)

var (
	categoryDescriptions map[int]string = make(map[int]string)
)

type NotFoundError struct {}

func (_ NotFoundError) Error() (error string) {
	return "Not found"
}

func pullSatelliteDescription(nssdc_url string) (satelliteDescription string, error error) {
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
		
		satelliteDescription = strings.Join(paragraphs, "\n")
	}

	return satelliteDescription, error
}

func pullCategoryInformation(ny20CategoryURL string) (categoryDescription string, error error) {
	response, error := http.Get("https://www.n2yo.com" + ny20CategoryURL)
	
	if error == nil {
		defer response.Body.Close()
		document, _ := goquery.NewDocumentFromReader(response.Body)
		the_table := document.Find("table").Text()
		title := document.Find("h1").Text()
		re := regexp.MustCompile("(.*\n)*" + title + "\\s*(?P<description>.*\n)(.*\n)*.*")
		categoryDescription = re.ReplaceAllString(the_table, "${description}")
	} else {
		categoryDescription = ""
		error = NotFoundError {}
	}

	return categoryDescription, error
}

func spawnRequests(startNoradID, endNoradID int, categoryChannel chan map[int]string, satelliteChannel chan satellite, finished chan bool) {
	interval, _ := time.ParseDuration("0.5s")
	
	for noradID:=startNoradID; noradID<=endNoradID; noradID++ {
		go pullSatelliteInfo(noradID, categoryChannel, satelliteChannel)
		time.Sleep(interval)
	}

	finalWait, _ := time.ParseDuration("10s")
	time.Sleep(finalWait)
	
	finished <- true

	return
}

func satelliteDescriptionAlreadyExists(noradID int) bool {
	path := satelliteDescriptionsDir + strconv.Itoa(noradID)
	_, err := os.Stat(path)
	
	return err == nil
}

func categoryAlreadyExists(categoryID int) bool {
	path := categoryDescriptionsDir + strconv.Itoa(categoryID)
	_, err := os.Stat(path)

	return err == nil
}

func pullSatelliteInfo(noradID int, categoryChannel chan map[int]string, satelliteChannel chan satellite) {
	response, error := http.Get(fmt.Sprintf("https://www.n2yo.com/satellite/?s=%d#results", noradID))
	
	satellite := satellite{noradID:noradID}
	
	if error == nil {
		defer response.Body.Close()
		document, _ := goquery.NewDocumentFromReader(response.Body)
		re := regexp.MustCompile("[[:digit:]]*$")
		
		document.Find(".arrow a").Each(
			func(i int, selection *goquery.Selection) {
				ny20CategoryURL, _ := selection.Attr("href")
				categoryID, _ := strconv.Atoi(re.FindString(ny20CategoryURL))
				
				if ! categoryAlreadyExists(categoryID) { 
					categoryDescription, _ := pullCategoryInformation(ny20CategoryURL)
					categoryMap := make(map[int]string)
					categoryMap[categoryID] = categoryDescription
					categoryChannel <- categoryMap
				}

				satellite.categories = append(satellite.categories, categoryID)
			})

		
		if ! satelliteDescriptionAlreadyExists(noradID) {
			document.Find("tbody a").Each(
				func(i int, selection *goquery.Selection) {
					nssdcURL, _ := selection.Attr("href")
					if strings.Contains(nssdcURL, "nssdc.gsfc.nasa.gov") {
						satelliteDescription, _ := pullSatelliteDescription(nssdcURL)
						satellite.description = satelliteDescription
						
					}
				})
		}
	}

	satelliteChannel <- satellite

	return
}

type satellite struct {
	noradID int
	description string
	categories []int
}

func categoryArrayToString(categories []int) (categoryString string) {
	for i, category := range categories {
		categoryString += strconv.Itoa(category)
		if i != len(categories) - 1 {
			categoryString +=  ","
		}
	}

	return
}

func main() {
	startNoradID := flag.Int("s", 0, "Norad ID to start at")
	endNoradID := flag.Int("e", 53000, "Norad ID to end at")
	flag.Parse()
	fmt.Println("Pulling satellites starting at noradID", *startNoradID, "and finishing with", *endNoradID)
	
	categoryChannel := make(chan map[int]string)
	satelliteChannel := make(chan satellite)
	finished := make(chan bool, 1)
	
	go spawnRequests(*startNoradID, *endNoradID, categoryChannel, satelliteChannel, finished)

	os.Mkdir(outputDir, 0755)
	os.Mkdir(satelliteDescriptionsDir, 0755)
	os.Mkdir(categoryDescriptionsDir, 0755)
	
	for {
		select {
		case categories := <-categoryChannel:
			var categoryArray []int

			for k, _ := range categories {
				categoryArray = append(categoryArray, k)
			}

			categoryString := categoryArrayToString(categoryArray)
			
			fmt.Println("Received categories:", categoryString)
			for index, description := range categories {
				path := categoryDescriptionsDir + strconv.Itoa(index)
				data := []byte(description)
				os.WriteFile(path, data, 0644)
			}
			
		case satellite := <-satelliteChannel:
			fmt.Println("Received satellite with noradID", satellite.noradID)
			if len(satellite.description) != 0 {
				path := satelliteDescriptionsDir + strconv.Itoa(satellite.noradID)
				data := []byte(satellite.description)
				os.WriteFile(path, data, 0644)
				fmt.Println("\tLoaded description")
			} else {
				fmt.Println("\tDescription exists on disk or doesn't exist online. Not saving.")
			}

			categoryString := categoryArrayToString(satellite.categories)

			os.WriteFile(satelliteDescriptionsDir + strconv.Itoa(satellite.noradID) + "-categories", []byte(categoryString), 0644)
			
			if len(categoryString) != 0 {
				fmt.Println("\tCategories:", categoryString)
			} else {
				fmt.Println("\tNo categories")
			}
			
		case <-finished:
			fmt.Println("Done")
			return
		}
	}
}
