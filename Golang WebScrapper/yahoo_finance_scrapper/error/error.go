package error

import (
	"fmt"
	"time"
)

func NoEmailConfigFound() {
	fmt.Println("No config file found...")
	fmt.Println("if you want to send an email..")
	//wait 2 seconds
	time.Sleep(1 * time.Second)
	fmt.Println("Please create a config file named config.email.env with the following format..")
	time.Sleep(1 * time.Second)
	fmt.Println("EMAIL=EMAIL_ADDRESS")
	fmt.Println("PASSWORD=PASSWORD")
	fmt.Println("SMTP_HOST=SMTP_HOST")
	fmt.Println("SMTP_PORT=SMTP_PORT")
}
