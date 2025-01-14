package file

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
)

func SaveDataToCSV(data [][]string, headers []string) {
	fmt.Println("Saving data to file... Removing old file if exists")
	os.Remove("Daily_Actives.csv")
	file, err := os.Create("Daily_Actives.csv")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)

	writer.Write(headers)

	writer.Flush()
	for _, value := range data {
		err := writer.Write(value)
		if err != nil {
			log.Fatal(err)
		}
	}
	writer.Flush()

	fmt.Println("Data saved to file")
}
