package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"github.com/gofiber/fiber/v2"
	"github.com/jcelliott/lumber"

)

const Version = "1.0.0"

type (
	Logger interface {
		Fatal(string, ...interface{})
		Error(string, ...interface{})
		Warn(string, ...interface{})
		Info(string, ...interface{})
		Debug(string, ...interface{})
		Trace(string, ...interface{})
	}

	Driver struct {
		mutex   sync.Mutex
		mutexes map[string]*sync.Mutex
		dir     string
		log     Logger
	}
)

type Options struct {
	Logger
}

func New(dir string, options *Options) (*Driver, error) {
	dir = filepath.Clean(dir)

	opts := Options{}

	if options != nil {
		opts = *options
	}

	if opts.Logger == nil {
		opts.Logger = lumber.NewConsoleLogger((lumber.INFO))
	}

	driver := Driver{
		dir:     dir,
		mutexes: make(map[string]*sync.Mutex),
		log:     opts.Logger,
	}

	if _, err := os.Stat(dir); err == nil {
		opts.Logger.Debug("Using '%s' (database already exists)\n", dir)
		return &driver, nil
	}

	opts.Logger.Debug("Creating the database at '%s'...\n", dir)
	return &driver, os.MkdirAll(dir, 0755)
}

func (d *Driver) Write(collection, resource string, v interface{}) error {
	if collection == "" {
		return fmt.Errorf("Missing collection - no place to save record!")
	}

	if resource == "" {
		return fmt.Errorf("Missing resource - unable to save record (no name)!")
	}

	mutex := d.getOrCreateMutex(collection)
	mutex.Lock()
	defer mutex.Unlock()

	dir := filepath.Join(d.dir, collection)
	fnlPath := filepath.Join(dir, resource+".json")
	tmpPath := fnlPath + ".tmp"

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	b, err := json.MarshalIndent(v, "", "\t")
	if err != nil {
		return err
	}

	b = append(b, byte('\n'))

	if err := ioutil.WriteFile(tmpPath, b, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, fnlPath)
}

func (d *Driver) Read(collection, resource string, v interface{}) error {
	if collection == "" {
		return fmt.Errorf("Missing collection - unable to read!")
	}

	if resource == "" {
		return fmt.Errorf("Missing resource - unable to read record (no name)!")
	}

	record := filepath.Join(d.dir, collection, resource + ".json") // Ensure only one .json extension

	if _, err := stat(record); err != nil {
		return err
	}

	b, err := ioutil.ReadFile(record)
	if err != nil {
		return err
	}

	return json.Unmarshal(b, &v)
}


func (d *Driver) ReadAll(collection string) ([]string, error) {

	if collection == "" {
		return nil, fmt.Errorf("Missing collection - unable to read")
	}
	dir := filepath.Join(d.dir, collection)

	if _, err := stat(dir); err != nil {
		return nil, err
	}

	files, _ := ioutil.ReadDir(dir)

	var records []string

	for _, file := range files {
		b, err := ioutil.ReadFile(filepath.Join(dir, file.Name()))
		if err != nil {
			return nil, err
		}

		records = append(records, string(b))
	}
	return records, nil
}

func (d *Driver) Delete(collection, resource string) error {

	path := filepath.Join(collection, resource)
	mutex := d.getOrCreateMutex(collection)
	mutex.Lock()
	defer mutex.Unlock()

	dir := filepath.Join(d.dir, path)

	switch fi, err := stat(dir); {
	case fi == nil, err != nil:
		return fmt.Errorf("unable to find file or directory named %v\n", path)

	case fi.Mode().IsDir():
		return os.RemoveAll(dir)

	case fi.Mode().IsRegular():
		return os.RemoveAll(dir + ".json")
	}
	return nil
}

func (d *Driver) getOrCreateMutex(collection string) *sync.Mutex {

	d.mutex.Lock()
	defer d.mutex.Unlock()
	m, ok := d.mutexes[collection]

	if !ok {
		m = &sync.Mutex{}
		d.mutexes[collection] = m
	}

	return m
}

func stat(path string) (fi os.FileInfo, err error) {
	if fi, err = os.Stat(path); os.IsNotExist(err) {
		fi, err = os.Stat(path + ".json")
	}
	return
}

type Address struct {
	City    string
	State   string
	Country string
	Pincode json.Number
}

type User struct {
	Name    string
	Age     json.Number
	Contact string
	Company string
	Address Address
}

func main() {
	app := fiber.New()
	dir := "./"

	db, err := New(dir, nil)
	if err != nil {
		fmt.Println("Error", err)
	}

	// employees := []User{
	// 	{"John", "23", "23344333", "Myrl Tech", Address{"bangalore", "karnataka", "india", "410013"}},
	// 	{"Paul", "25", "23344333", "Google", Address{"san francisco", "california", "USA", "410013"}},
	// 	{"Robert", "27", "23344333", "Microsoft", Address{"bangalore", "karnataka", "india", "410013"}},
	// 	{"Vince", "29", "23344333", "Facebook", Address{"bangalore", "karnataka", "india", "410013"}},
	// 	{"Neo", "31", "23344333", "Remote-Teams", Address{"bangalore", "karnataka", "india", "410013"}},
	// 	{"Albert", "32", "23344333", "Dominate", Address{"bangalore", "karnataka", "india", "410013"}},
	// }

	// for _, value := range employees {
	// 	db.Write("users", value.Name, User{
	// 		Name:    value.Name,
	// 		Age:     value.Age,
	// 		Contact: value.Contact,
	// 		Company: value.Company,
	// 		Address: value.Address,
	// 	})
	// }

	app.Post("/addUser", func(c *fiber.Ctx) error {
		var user User

		if err := c.BodyParser(&user); err != nil {
			return c.Status(400).SendString("Error parsing request body")
		}

		if err := db.Write("users", user.Name, user); err != nil {
			return c.Status(500).SendString("Error saving user data")
		}

		return c.Status(201).JSON(user)
	})

	app.Delete("/deleteUser/:name", func(c *fiber.Ctx) error {
		name := c.Params("name")
	
		if name == "" {
			return c.Status(400).SendString("Name parameter is required")
		}
	
		if err := db.Delete("users", name); err != nil {
			return c.Status(500).SendString("Error deleting user data")
		}
	
		return c.SendString("User deleted successfully")
	})


	app.Delete("/deleteAllUsers", func(c *fiber.Ctx) error {
		if err := db.Delete("users", ""); err != nil {
			return c.Status(500).SendString("Error deleting all user data")
		}
	
		return c.SendString("All users deleted successfully")
	})


	app.Get("/getUser/:name", func(c *fiber.Ctx) error {
		name := c.Params("name")
	
		if name == "" {
			return c.Status(400).SendString("Name parameter is required")
		}
	
		var user User
		if err := db.Read("users", name, &user); err != nil {
			// Log the error and return a detailed message
			return c.Status(500).SendString(fmt.Sprintf("Error retrieving user data: %v", err))
		}
	
		return c.JSON(user)
	})

	app.Get("/getAllUsers", func(c *fiber.Ctx) error {
		records, err := db.ReadAll("users")
		if err != nil {
			return c.Status(500).SendString("Error retrieving all users")
		}
	
		var allUsers []User
		for _, record := range records {
			var user User
			if err := json.Unmarshal([]byte(record), &user); err != nil {
				return c.Status(500).SendString("Error parsing user data")
			}
			allUsers = append(allUsers, user)
		}
	
		return c.JSON(allUsers)
	})
	
	


	app.Listen(":3000")

	// records, err := db.ReadAll("users")
	// if err != nil {
	// 	fmt.Println("Error", err)
	// }
	// fmt.Println(records)

	// allusers := []User{}

	// for _, f := range records {
	// 	employeeFound := User{}
	// 	if err := json.Unmarshal([]byte(f), &employeeFound); err != nil {
	// 		fmt.Println("Error", err)
	// 	}
	// 	allusers = append(allusers, employeeFound)
	// }
	// fmt.Println((allusers))

	// if err := db.Delete("users", "John"); err != nil {
	// 	fmt.Println("Error", err)
	// }

	// if err := db.Delete("users", ""); err != nil {
	// 	fmt.Println("Error", err)
	// }
}
