/*
Copyright © 2022 Joker

*/
package cmd

import (
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v4"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	echoSwagger "github.com/swaggo/echo-swagger"
	"os"
	"path/filepath"
	_ "sysafari.com/customs/cguard/docs"
	"sysafari.com/customs/cguard/logging"
	"sysafari.com/customs/cguard/lwt"
	"sysafari.com/customs/cguard/rabbit"
	"time"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "cguard",
	Short: "Generate various documents required by the customs declaration system",
	Long: `In order to ensure the normal customs declaration business, 
the data files or view files of various report types that need to be generated. For example:
1. LWT documents that need to be submitted when customs declaration encounters inspection.
..
`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: func(cmd *cobra.Command, args []string) {
		// async lwt request consumer
		go LwtConsumerStart()

		// Web
		echoRoutes()

	},
}

// echoRoutes Set echo routes
// @title LWT web service
// @version 1.0
// @description This is a simple web service that provides download lwt file
// @termsOfService http://swagger.io/terms/

// @contact.name Joker
// @contact.email ljr@y-clouds.com

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host localhost:7004
// @BasePath /v2
func echoRoutes() {
	e := echo.New()
	// swagger
	e.GET("/swagger/*", echoSwagger.WrapHandler)

	// api exp:
	// http://localhost:{port}/lwt/OP210603005_20220909153131.xlsx?download=1
	e.GET("/lwt/:filename", lwt.DownloadLwtExcel)

	port := viper.GetString("port")
	if port == "" {
		port = "1324"
	}

	fmt.Printf("Rattler server started: %v", e.Start(":"+port))
}

// LwtConsumerStart Start lwt request consumer
func LwtConsumerStart() {
	// Set global db connection
	initGlobalDatabaseConnection()

	// start amqp consumer
	listenerForLwtRequest()
}

// initGlobalDatabaseConnection sets the global database connection
func initGlobalDatabaseConnection() {
	fmt.Println("init sql connection ....")
	db, err := sqlx.Open("mysql", viper.GetString("mysql.url"))

	if err != nil {
		panic(err)
	}
	// See "Important settings" section.
	db.SetConnMaxLifetime(time.Minute * 3)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)

	lwt.Db = db
}

// Generate lwt request queue listener
func listenerForLwtRequest() {
	rbmq := &rabbit.Rabbit{
		Url:          viper.GetString("rabbitmq.url"),
		Exchange:     viper.GetString("rabbitmq.exchange"),
		ExchangeType: viper.GetString("rabbitmq.exchange-type"),
		Queue:        viper.GetString("rabbitmq.queue.lwt-req"),
	}

	log.Infof("Starting ... LWT request consumer: %v ", rbmq)
	rabbit.Consume(rbmq, lwt.GenerateLWTExcel)
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", ".cguard.yaml", "config file (default is .cguard.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	//rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	fmt.Println("Init vipper ...")
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".cguard" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".cguard")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
	// init logging
	initLogging()
}

func initLogging() {
	path, _ := os.Executable()
	_, exec := filepath.Split(path)
	fmt.Println(exec)
	logfile := fmt.Sprintf("%s/%s.log", viper.GetString("log.log-base"), exec)

	logging.InitLog(logfile, viper.GetString("log.level"))
}
