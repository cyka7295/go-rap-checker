package Core

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	cat Catalog
	items []Item
	assettypes = []string{
		"Hat",
		"Gear",
		"Face",
		"HairAccessory",
		"FaceAccessory",
		"NeckAccessory",
		"ShoulderAccessory",
		"FrontAccessory",
		"BackAccessory",
		"WaistAccessory",
	}
	values = make(map[int64]int32)
)

type Catalog struct {
	Items []Item
}

type Item struct {
	AssetID int64
	Name string
	Value int32
	RAP   int32
}

type ItemData struct {
	Data []struct {
		ID	int64 `json:"id"`
		InstanceID int64 `json:"instanceId"`
	} `json:"data"`
}

type Resp struct {
	NextPageCursor string `json:"nextPageCursor"`
	Data []struct {
		AssetID int64 `json:"assetId"`
		Name string `json:"name"`
		RAP  int32  `json:"recentAveragePrice"`
	} `json:"data"`
}

func init() {
	// rap & value for priv inventories is taken from rolimons every 5 mins

	loaded := make(chan int)
	go func() {
		for {
			// name = [0], RAP = [8], value = [16]
			var catalog map[string][]interface{}

			res, _ := http.Get("https://www.rolimons.com/catalog")
			body, _ := ioutil.ReadAll(res.Body)
			res.Body.Close()

			items := strings.Split(strings.Split(string(body), "item_details = ")[1], ";")[0]
			json.Unmarshal([]byte(items), &catalog)
			for id, values := range catalog {
				var rap, val int32
				x, _ := strconv.ParseInt(id, 10, 64)
				rap = int32(values[8].(float64))
				if values[16] == nil {
					val = rap
				} else {
					val = int32(values[16].(float64))
				}
				cat.Items = append(cat.Items, Item{
					AssetID: x,
					Name: values[0].(string),
					RAP: rap,
					Value: val,
				})
			}
			for _, item := range cat.Items {
				values[item.AssetID] = item.Value
			}

			loaded <- 1

			// refreshing value & RAP
			time.Sleep(time.Minute * 5)
		}
	}()
	<-loaded
}

func GrabItem(assetid, userid int64) (count int32) {
	start:
	res, err := http.Get(fmt.Sprintf("https://inventory.roblox.com/v1/users/%d/items/Asset/%d", assetid, userid))
	if err != nil {
		goto start
	}
	defer res.Body.Close()
	var data *ItemData
	err = json.NewDecoder(res.Body).Decode(&data)
	if err != nil {
		goto start
	}
	for range data.Data {
		count++
	}
	return
}


func GetRAPFromAssetType(userid int64, rap, val *int32, assettype, cursor string, wg *sync.WaitGroup) {
	start:
	res, err := http.Get(fmt.Sprintf("https://inventory.roblox.com/v1/users/%d/assets/collectibles?limit=100&assettype=%s&cursor=" + cursor, userid, assettype))
	if err != nil {
		goto start
	}
	defer res.Body.Close()
	var resp *Resp
	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		goto start
	}
	if resp.NextPageCursor != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			GetRAPFromAssetType(userid, rap, val, assettype, resp.NextPageCursor, wg)
		}()
	}

	for _, item := range resp.Data {
		*rap += item.RAP
		*val += values[item.AssetID]
	}
}

func GetRAP(id int64) (rap, val int32) {
	var wg sync.WaitGroup
	wg.Add(len(assettypes))

	for _, asset := range assettypes {
		asset := asset
		go func() {
			defer wg.Done()
			GetRAPFromAssetType(id, &rap, &val, asset, "", &wg)
		}()

	}
	wg.Wait()
	return
}

func GetPrivRAP(id int64) (rap, val int32, items []string) {
	var wg sync.WaitGroup

	wg.Add(len(cat.Items))
	for _, item := range cat.Items {
		item := item
		go func() {
			defer wg.Done()
			if y := GrabItem(id, item.AssetID); y > 0 {
				val += item.Value * y
				rap += item.RAP * y
				for i := 0; i < int(y); i++ {
					items = append(items, item.Name)
				}
			}
		}()
	}
	wg.Wait()
	return
}
