package main

import (
	"flag"
	"path"
	"time"
	"io/ioutil"
	"os"
	"fmt"
	"net/http"
	"github.com/PuerkitoBio/goquery"
	"strings"
	"regexp"
	"strconv"
)

const (
	outputDir = "data/"
	satelliteDescriptionsDir = outputDir + "satellite-descriptions/"
	satelliteCategoriesDir = outputDir + "satellite-categories/"
	categoryDescriptionsDir = outputDir + "category-descriptions/"
	imagesDir = outputDir + "images/"
)

var (
	categoryDescriptions map[int]string = make(map[int]string)
)

type image struct {
	data []byte
	noradID int
	basename string
}

type NotFoundError struct {}

func (_ NotFoundError) Error() (error string) {
	return "Not found"
}

func pullSatelliteDescription(noradID int, nssdcURL string) (satelliteDescription string, images []image, error error) {
	response, error := http.Get(nssdcURL)

	if error == nil {
		defer response.Body.Close()
		document, _ := goquery.NewDocumentFromReader(response.Body)
		
		var paragraphs []string
		
		document.Find(".urone p").Each(
			func(i int, selection *goquery.Selection) {
				text := selection.Text()
				if len(strings.TrimSpace(text)) != 0 {
					paragraphs = append(paragraphs, text)
				}
			})

		document.Find("#leftcontent img").Each(
			func(i int, selection *goquery.Selection) {
				src, _ := selection.Attr("src")
				if ! imageAlreadyExists(path.Base(src), noradID) {
					imageResponse, error := http.Get(src)
					if error == nil {
						defer imageResponse.Body.Close()
						basename := path.Base(src)
						data, _ := ioutil.ReadAll(imageResponse.Body)
						images = append(images, image{data: data, noradID: noradID, basename: basename})
					}
				}
			})
		
		satelliteDescription = strings.Join(paragraphs, "\n")
	}

	return
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

func spawnRequests(startNoradID, endNoradID int, categoryChannel chan map[int]string, satelliteChannel chan satellite, imagesChannel chan image, finished chan bool) {
	interval, _ := time.ParseDuration("0.3s")
	
	for noradID:=startNoradID; noradID<=endNoradID; noradID++ {
		go pullSatelliteInfo(noradID, categoryChannel, satelliteChannel, imagesChannel)
		time.Sleep(interval)
	}

	finalWait, _ := time.ParseDuration("10s")
	time.Sleep(finalWait)
	
	finished <- true

	return
}

func imageAlreadyExists(basename string, noradID int) bool {
	dir := imagesDir + strconv.Itoa(noradID) + "/"
	images, _ := os.ReadDir(dir)
	
	for _, image := range images {
		if basename == image.Name() {
			return true
		}
	}

	return false
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

func pullSatelliteInfo(noradID int, categoryChannel chan map[int]string, satelliteChannel chan satellite, imagesChannel chan image) {
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
						// Don't need to pass noradID, it can be specified here
						satelliteDescription, images, _ := pullSatelliteDescription(noradID, nssdcURL)
						satellite.description = satelliteDescription
						for _, image := range images {
							imagesChannel <- image
						}
					}
				})
			
		}
	}

	imageURL := fmt.Sprintf("https://www.heavens-above.com/images/satellites/%05d.jpg", noradID)
	if ! imageAlreadyExists(path.Base(imageURL), noradID) {
		imageData, error := http.Get(imageURL)
		if error == nil && imageData.StatusCode != 404 {
			defer imageData.Body.Close()
			data, _ := ioutil.ReadAll(imageData.Body)
			imagesChannel <- image{ data: data, noradID: noradID, basename: path.Base(imageURL) }
		}
	}

	satelliteChannel <- satellite

	return
}

type satellite struct {
	noradID int
	description string
	images []image
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
	imagesChannel := make(chan image)
	finished := make(chan bool, 1)
	
	go spawnRequests(*startNoradID, *endNoradID, categoryChannel, satelliteChannel, imagesChannel, finished)

	os.Mkdir(outputDir, 0755)
	os.Mkdir(satelliteDescriptionsDir, 0755)
	os.Mkdir(categoryDescriptionsDir, 0755)
	os.Mkdir(satelliteCategoriesDir, 0755)
	os.Mkdir(imagesDir, 0755)
	
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
			if len(satellite.description) != 0 {
				fmt.Println("Received new description for satellite with noradID", satellite.noradID)
				path := satelliteDescriptionsDir + strconv.Itoa(satellite.noradID)
				data := []byte(satellite.description)
				os.WriteFile(path, data, 0644)
			}
			

			if len(satellite.categories) != 0 {
				categoryString := categoryArrayToString(satellite.categories)
				os.WriteFile(satelliteCategoriesDir + strconv.Itoa(satellite.noradID), []byte(categoryString), 0644)
			}

		case image := <- imagesChannel:
			fmt.Println("Received image from satellite with noradID", image.noradID)
			noradIDString := strconv.Itoa(image.noradID)
			specificOutputDir := imagesDir + noradIDString + "/"
			os.Mkdir(specificOutputDir, 0755)
			os.WriteFile(specificOutputDir + image.basename, image.data, 0644)
			
		case <-finished:
			fmt.Println("Done")
			return
		}
	}
}
