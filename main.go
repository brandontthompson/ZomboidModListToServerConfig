package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gosuri/uiprogress"
	"golang.org/x/net/html"
)

func getAttribute(node *html.Node, tag string) (string, bool) {
	for _, attr := range node.Attr {
		if attr.Key == tag {
			return attr.Val, true
		}
	}
	return "", false
}

func hasAttr(node *html.Node, id string, attr string) bool {
	if node.Type != html.ElementNode {
		return false
	}

	attr, ok := getAttribute(node, attr)
	if ok && attr == id {
		return true
	}

	return false
}

func traverse(node *html.Node, id string, attr string) *html.Node {
	if hasAttr(node, id, attr) {
		return node
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		result := traverse(child, id, attr)
		if result != nil {
			return result
		}
	}
	return nil
}

func doTraverse(node *html.Node, data *[]html.Node, id string, attr string) {

	var traverse func(node *html.Node, attr string) *html.Node

	traverse = func(node *html.Node, attr string) *html.Node {
		for child := node.FirstChild; child != nil; child = child.NextSibling {

			if hasAttr(child, id, attr) {
				*data = append(*data, *child)
			}

			result := traverse(child, attr)
			if result != nil {
				return result
			}

		}

		return nil
	}

	traverse(node, attr)
}

func getContent(node *html.Node) string {
	return node.FirstChild.Data
}

func getFirstElementByAttr(node *html.Node, id string, attr string) *html.Node {
	return traverse(node, id, attr)
}

func renderNode(node *html.Node) string {
	var buf bytes.Buffer
	writer := io.Writer(&buf)
	err := html.Render(writer, node)

	if err != nil {
		return ""
	}

	return buf.String()
}

func loadUrlContent(url string) []byte {
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println(err)
		return nil
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
		return nil
	}

	return body
}

func parseHtmlContent(b *[]byte) *html.Node {
	node, err := html.Parse(strings.NewReader(string(*b)))
	if err != nil {
		fmt.Println("Error parsing string", err)
	}
	return node
}

func produceVariants(str string) []string {
	str = strings.ToLower(str)
	variants := []string{str, strings.Replace(str, " ", "", -1), strings.Replace(str, ":", "", -1)}
	return variants
}

func findVariant(str string, variants []string) string {
	for _, variant := range variants {
		if strings.Contains(str, variant) {
			return variant
		}
	}
	return ""
}

func stripParse(str string, variants []string) []string {
	lowerStr := strings.ToLower(str)
	a := strings.Split(str, str[:strings.Index(lowerStr, findVariant(lowerStr, variants))])[1]
	a = StripHTMLTags(a)
	alower := strings.ToLower(a)
	return parseValues(a, findVariant(alower, variants))
}

func processModIdentifier(buff string) ([]string, []string) {

	modvariants := produceVariants("Mod ID:")
	mapvariants := produceVariants("Map Folder:")

	return stripParse(buff, modvariants), stripParse(buff, mapvariants)
}

func parseValues(str, prefix string) []string {
	strLower := strings.ToLower(str)
	re := regexp.MustCompile(fmt.Sprintf(`%s\s+(.+)\s+`, prefix))
	matches := re.FindAllStringSubmatch(strLower, -1)

	var values []string
	for _, match := range matches {
		value := strings.TrimSpace(str[strings.Index(strLower, match[1]) : strings.Index(strLower, match[1])+len(match[1])])
		values = append(values, value)
	}

	return values
}

func StripHTMLTags(input string) string {
	re := regexp.MustCompile("<[^>]*>")
	stripped := re.ReplaceAllString(input, "\n")
	return stripped
}

func reservedOrPlaceholder(placeholder *[]reservedPlaceholder, checkItem []string, index int, t string, iid string, ittl string) string {
	if len(checkItem) > 1 {
		*placeholder = append(*placeholder, reservedPlaceholder{
			placeholderType: t,
			reservedIndex:   index,
			options:         checkItem,
			itemId:          iid,
			itemTitle:       ittl,
		})
	}
	return checkItem[0]
}

type reservedPlaceholder struct {
	placeholderType string
	options         []string
	reservedIndex   int
	itemTitle       string
	itemId          string
}

func createConfigOutput(workshop *[]string, mod *[]string, m string) string {
	return "WorkshopItems=" + strings.Join(*workshop, ";") + ";\nMods=" + strings.Join(*mod, ";") + ";\nMap=" + m + ";"
}

func main() {
	fmt.Println("Enter the URL of the workshop collection you want to parse:")
	//reader := bufio.NewReader(os.Stdin)
	//inp, err := reader.ReadString('\n')
	var inp string
	_, err := fmt.Scanln(&inp)
	if err != nil {
		fmt.Println("An error occurred while reading input. Please try again", err)
		return
	}
	url := inp
	body := loadUrlContent(url)

	fmt.Println("Parsing Workshop collection: " + url + "\n")
	node := parseHtmlContent(&body)
	var data []html.Node
	modList := getFirstElementByAttr(node, "workshopItemTitle", "class")
	doTraverse(node, &data, "collectionItem", "class")

	var steps = []string{"scanning for collection items...", "fetching workshop content", "parsing workshop content", "collecting mod details"}
	bar := uiprogress.AddBar(len(data) * len(steps)).AppendCompleted().PrependElapsed()

	bar.PrependFunc(func(b *uiprogress.Bar) string {
		return steps[(b.Current()-1)%len(steps)]
	})

	uiprogress.Start()
	var workshopIds []string
	var modIds []string
	var mapIds []string
	var mapId string
	var reservedIndexes []reservedPlaceholder

	fmt.Println("Parsing mod list: " + getContent(modList))
	for i := 0; i < len(data); i++ {
		bar.Incr()
		title := getContent(getFirstElementByAttr(&data[i], "workshopItemTitle", "class"))
		steps[1], steps[2] = "fetching: "+title, "parsing: "+title
		contentId, _ := getAttribute(&data[i], "id")
		workshopId := strings.Split(contentId, "_")[1]

		u := "https://steamcommunity.com/sharedfiles/filedetails/?id=" + workshopId

		bar.Incr()
		b := loadUrlContent(u)

		bar.Incr()
		n := parseHtmlContent(&b)

		bar.Incr()
		desc := getFirstElementByAttr(n, "highlightContent", "id")
		modId, mpId := processModIdentifier(renderNode(desc))
		workshopIds = append(workshopIds, workshopId)
		if len(mpId) > 0 {
			mapIds = append(mapIds[:], mpId[:]...)
		}
		if len(modId) > 0 {
			modIds = append(modIds, reservedOrPlaceholder(&reservedIndexes, modId, len(modIds), "mod", workshopId, title))
		}
		time.Sleep(300 * time.Millisecond)
	}
	mapIds = append(mapIds, "")

	bar.AppendCompleted()
	uiprogress.Stop()

	for i := 0; i < len(reservedIndexes); i++ {
		res := reservedIndexes[i]

		fmt.Println("Multiple " + res.placeholderType + "s for " + res.itemTitle + " workshop item " + res.itemId + " please select one to enable")

		for j := 0; j < len(res.options); j++ {
			fmt.Println(j, ": ", res.options[j])
		}

		var input int
		_, err := fmt.Scanln(&input)
		if err != nil {
			fmt.Println("An error occurred while reading input. Please try again", err)
			return
		}
		modIds[res.reservedIndex] = res.options[input]
	}

	if len(mapIds) > 0 {
		fmt.Println("Multiple maps please select one to enable")
		for i := 0; i < len(mapIds); i++ {
			fmt.Println(i, ": ", mapIds[i])

		}
		var input int
		_, err := fmt.Scanln(&input)
		if err != nil {
			panic(err)
		}
		mapId = mapIds[input]
	}

	fmt.Println(createConfigOutput(&workshopIds, &modIds, mapId))

	fmt.Println("Press enter to exit")
	fmt.Scanln()

}
