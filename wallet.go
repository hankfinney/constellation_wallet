package main

import (
	"fmt"
	"os"
	"time"
)

func (a *WalletApplication) TempPrintCreds() {
	fmt.Println("address: ", a.wallet.Address, "alias: ", a.wallet.WalletAlias, "keyStorePass: ", os.Getenv("CL_STOREPASS"), "keyPass: ", os.Getenv("CL_KEYPASS"), "key: ", a.paths.EncPrivKeyFile)
}

/* Database Model is located in models.go */

func (a *WalletApplication) ImportWallet(keystorePath, keystorePassword, keyPassword, alias string) bool {

	if !a.TransactionFinished {
		a.log.Warn("Cannot Import wallet in a pending transaction.")
		a.LoginError("Cannot import a new wallet while there's a pending transaction.")
		return false
	}

	if !a.passwordsProvided(keystorePassword, keyPassword, alias) {
		a.log.Warnln("One or more passwords were not provided.")
		return false
	}

	os.Setenv("CL_STOREPASS", keystorePassword)
	os.Setenv("CL_KEYPASS", keyPassword)

	a.wallet = Wallet{
		KeyStorePath: keystorePath,
		WalletAlias:  alias}

	a.wallet.Address = a.GenerateDAGAddress()
	a.KeyStoreAccess = a.WalletKeystoreAccess()

	if a.KeyStoreAccess {
		if !a.DB.NewRecord(&a.wallet) {
			keystorePasswordHashed, err := a.GenerateSaltedHash(keystorePassword)
			if err != nil {
				a.log.Errorf("Unable to generate password hash. Reason: ", err)
				a.LoginError("Unable to generate password hash.")
				return false
			}

			keyPasswordHashed, err := a.GenerateSaltedHash(keyPassword)
			if err != nil {
				a.log.Errorf("Unable to generate password hash. Reason: ", err)
				a.LoginError("Unable to generate password hash.")
				return false
			}

			a.TempPrintCreds()

			if err := a.DB.Create(&a.wallet).Error; err != nil {
				a.log.Errorf("Unable to create database object for the imported wallet. Reason: ", err)
				a.LoginError("Unable to create database object for the imported wallet. Maybe it's already imported? Try to login.")
				return false
			}

			if err := a.DB.Where("wallet_alias = ?", a.wallet.WalletAlias).First(&a.wallet).Updates(&Wallet{KeyStorePath: keystorePath, KeystorePasswordHash: keystorePasswordHashed, KeyPasswordHash: keyPasswordHashed}).Error; err != nil {
				a.log.Errorf("Unable to query database object for the imported wallet. Reason: ", err)
				a.LoginError("Unable to query database object for the imported wallet.")
				return false
			}

			a.UserLoggedIn = false
			a.NewUser = true
			a.initWallet(keystorePath)

			return true

		} else if a.DB.NewRecord(&a.wallet) {
			a.DB.First(&a.wallet)

			a.TempPrintCreds()

			a.UserLoggedIn = false
			a.NewUser = false
			a.initWallet(a.wallet.KeyStorePath)
			return true
		}
	}

	return false
}

// CreateUser is called when creating a new wallet in frontend component Login.vue
func (a *WalletApplication) CreateWallet(keystorePath, keystorePassword, keyPassword, alias string) bool {

	if !a.TransactionFinished {
		a.log.Warn("Cannot Create wallet in a pending transaction.")
		a.LoginError("Cannot create a new wallet while there's a pending transaction.")
		return false
	}
	if !a.passwordsProvided(keystorePassword, keyPassword, alias) {
		a.log.Warnln("One or more passwords were not provided.")
		return false
	}

	if alias == "" {
		alias = a.wallet.WalletAlias
	}

	os.Setenv("CL_STOREPASS", keystorePassword)
	os.Setenv("CL_KEYPASS", keyPassword)

	keystorePasswordHashed, err := a.GenerateSaltedHash(keystorePassword)
	if err != nil {
		a.log.Errorf("Unable to generate password hash. Reason: ", err)
		a.sendError("Unable to generate password hash. Reason: ", err)
		return false
	}

	keyPasswordHashed, err := a.GenerateSaltedHash(keyPassword)
	if err != nil {
		a.log.Errorf("Unable to generate password hash. Reason: ", err)
		a.sendError("Unable to generate password hash. Reason: ", err)
		return false
	}

	a.wallet = Wallet{
		KeyStorePath:         keystorePath,
		KeystorePasswordHash: keystorePasswordHashed,
		KeyPasswordHash:      keyPasswordHashed,
		WalletAlias:          alias}

	if err := a.DB.Create(&a.wallet).Error; err != nil {
		a.log.Errorf("Unable to create database object for new wallet. Reason: ", err)
		a.LoginError("Unable to create new wallet. Alias already exists.")
		return false
	}

	if err := a.DB.Where("wallet_alias = ?", alias).First(&a.wallet).Updates(&Wallet{KeyStorePath: keystorePath, KeystorePasswordHash: keystorePasswordHashed, KeyPasswordHash: keyPasswordHashed}).Error; err != nil {
		a.log.Errorf("Unable to query database object for new wallet after wallet creation. Reason: ", err)
		a.sendError("Unable to query database object for new wallet after wallet creation. Reason: ", err)
		return false
	}

	a.CreateEncryptedKeyStore()

	a.wallet.Address = a.GenerateDAGAddress()

	if err := a.DB.Model(&a.wallet).Where("wallet_alias = ?", a.wallet.WalletAlias).Update("Address", a.wallet.Address).Error; err != nil {
		a.log.Errorf("Unable to update db object new wallet, with the DAG address. Reason: ", err)
		a.sendError("Unable to update db object new wallet, with the DAG address. Reason. Reason: ", err)
	}
	a.KeyStoreAccess = a.WalletKeystoreAccess()

	if a.KeyStoreAccess {
		a.paths.LastTXFile = a.TempFileName("tx-", "")
		a.paths.PrevTXFile = a.TempFileName("tx-", "")
		a.paths.EmptyTXFile = a.TempFileName("tx-", "")

		err := a.createTXFiles()
		if err != nil {
			a.log.Fatalln("Unable to create TX files. Check fs permissions. Reason: ", err)
			a.sendError("Unable to create TX files. Check fs permissions. Reason: ", err)
		}

		if err := a.DB.Where("wallet_alias = ?", a.wallet.WalletAlias).First(&a.wallet).Update("Path", Path{LastTXFile: a.paths.LastTXFile, PrevTXFile: a.paths.PrevTXFile, EmptyTXFile: a.paths.EmptyTXFile}).Error; err != nil {
			a.log.Errorf("Unable to update the DB record with the tmp tx-paths. Reason: ", err)
			a.sendError("Unable to update the DB record with the tmp tx-paths. Reason: ", err)
		}

		a.UserLoggedIn = false
		a.FirstTX = true
		a.NewUser = true

		a.initNewWallet()

		return true
	}

	return false
}

// initWallet initializes a new wallet. This is called from login.vue
// only when a new wallet is created.
func (a *WalletApplication) initNewWallet() {

	a.StoreImagePathInDB("faces/face-0.jpg")

	//a.initTransactionHistory()
	a.passKeysToFrontend()

	if !a.WidgetRunning.DashboardWidgets {
		a.initDashboardWidgets()
	}
	a.log.Infoln("A New wallet has been created successfully!")
}

// initExistingWallet queries the database for the user wallet and pushes
// the information to the front end components.
func (a *WalletApplication) initWallet(keystorePath string) {

	a.paths.EncPrivKeyFile = keystorePath

	a.initTXFilePath() // Update paths from DB.
	a.initTXFromDB()   // Disregard upon import

	if !a.WidgetRunning.DashboardWidgets {
		a.initDashboardWidgets()
	}
	if !a.WidgetRunning.PassKeysToFrontend {
		a.passKeysToFrontend()
	}

	a.log.Infoln("User has logged into the wallet")

}

func (a *WalletApplication) initDashboardWidgets() {
	// Initializes a struct containing all Chart Data on the dashboard
	chartData := a.ChartDataInit()

	// Below methods are continously updating the client side modules.
	a.nodeStats(chartData)
	a.txStats(chartData)
	a.networkStats(chartData)
	a.blockAmount()
	a.tokenAmount()
	a.pricePoller()

	a.WidgetRunning.DashboardWidgets = true
}

func (a *WalletApplication) createTXFiles() error {
	files := []string{a.paths.LastTXFile, a.paths.PrevTXFile, a.paths.EmptyTXFile}

	for _, f := range files {
		file, err := os.Create(f)
		if err != nil {
			return err
		}
		defer file.Close()
	}
	return nil
}

// ImportKeys is called from the frontend to use a file dialog to select key file.
func (a *WalletApplication) ImportKeys() error {
	filename := a.RT.Dialog.SelectFile()
	a.log.Info("Path to keys that user wants to import: " + filename)
	return nil
}

// ExportKeys is called from the frontend to use a file dialog to select directory
// where user wants to export the keys to.
func (a *WalletApplication) ExportKeys() error {
	filename := a.RT.Dialog.SelectDirectory()
	a.log.Info("File user wants to save to: " + filename)
	return nil
}

func (a *WalletApplication) initTXFromDB() {
	transactions := &a.wallet.TXHistory
	if err := a.DB.Model(&a.wallet).Where("alias = ?", a.wallet.WalletAlias).Association("TXHistory").Find(&transactions).Error; err != nil {
		a.log.Error("Unable to initialize historic transactions from DB. Reason: ", err)
		a.sendError("Unable to initialize historic transactions from DB. Reason: ", err)
		return
	}

	for i := range a.wallet.TXHistory {
		if !a.wallet.TXHistory[i].Failed {
			a.RT.Events.Emit("new_transaction", &a.wallet.TXHistory[i]) // Pass the tx to the frontend as a new transaction.
		}
	}

}

func (a *WalletApplication) initTXFilePath() {
	paths := &a.wallet.Path
	if err := a.DB.Model(&a.wallet).Where("alias = ?", a.wallet.WalletAlias).Association("Path").Find(&paths).Error; err != nil {
		a.log.Error("Unable to initialize TX filepaths. Reason: ", err)
		a.sendError("Unable to initialize TX filepaths. Reason: ", err)
		return
	}
	a.paths.LastTXFile = a.wallet.Path.LastTXFile
	a.paths.PrevTXFile = a.wallet.Path.PrevTXFile
	a.paths.EmptyTXFile = a.wallet.Path.EmptyTXFile

}

func (a *WalletApplication) initTXFromBlockExplorer() {
	// TODO
}

// PassKeysToFrontend emits the keys to the settings.Vue component on a
// 5 second interval
func (a *WalletApplication) passKeysToFrontend() {
	if a.paths.EncPrivKeyFile != "" && a.wallet.Address != "" {
		go func() {
			for {
				a.RT.Events.Emit("wallet_keys", a.wallet.Address)
				time.Sleep(5 * time.Second)
			}
		}()
		a.WidgetRunning.PassKeysToFrontend = true
	} else {
		a.WidgetRunning.PassKeysToFrontend = false
	}
}

func (a *WalletApplication) passwordsProvided(keystorePassword, keyPassword, alias string) bool {
	if a.paths.EncPrivKeyFile == "" {
		a.LoginError("Please provide a valid path to your KeyStore file.")
		a.TempPrintCreds()
		return false
	} else if keystorePassword == "" {
		a.LoginError("Please provide a Key Store password.")
		a.TempPrintCreds()
		return false
	} else if keyPassword == "" {
		a.LoginError("Please provide a Key Password.")
		a.TempPrintCreds()
		return false
	} else if alias == "" {
		a.LoginError("An Alias has not been provided.")
		a.TempPrintCreds()
		return false
	}
	return true
}
