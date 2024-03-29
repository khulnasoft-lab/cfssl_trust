package cli

import (
	"crypto/x509"
	"database/sql"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/khulnasoft-lab/cfssl/helpers"
	"github.com/khulnasoft-lab/cfssl_trust/model/certdb"
	_ "github.com/mattn/go-sqlite3" // load sql driver
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import certificates into the database.",
	Long:  "Import certificates into the database, marking them under a release as needed.",
	Run:   importer,
}

func init() {
	rootCmd.AddCommand(importCmd)
}

func importCertificate(tx *sql.Tx, cert *x509.Certificate, rel *certdb.Release) error {
	fmt.Printf("- importing serial %s SKI %x\n", cert.SerialNumber, cert.SubjectKeyId)
	c := certdb.NewCertificate(cert)
	_, err := certdb.Ensure(c, tx)
	if err != nil {
		return err
	}

	aia := certdb.NewAIA(c)
	if aia != nil {
		_, err = certdb.Ensure(aia, tx)
		if err != nil {
			return err
		}
	}

	// The rest of the function deals with inserting the
	// certificate into the relevant release table. The assumption
	// here is that the release exists in the DB.
	if rel == nil {
		return nil
	}

	cr := certdb.NewCertificateRelease(c, rel)
	_, err = certdb.Ensure(cr, tx)
	return err

}

func importer(cmd *cobra.Command, args []string) {
	dbPath := viper.GetString("database.path")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[!] %s\n", err)
		os.Exit(1)
	}

	tx, err := db.Begin()
	if err != nil {
		if err != nil {
			fmt.Fprintf(os.Stderr, "[!] %s\n", err)
			os.Exit(1)
		}
	}
	defer func() {
		if err == nil {
			err = tx.Commit()
			if err != nil {
				fmt.Fprintf(os.Stderr, "[!] failed to commit transaction: %s\n", err)
				os.Exit(1)
			}
		} else {
			tx.Rollback()
		}
	}()

	var rel *certdb.Release
	if bundleRelease != "" {
		rel, err = certdb.NewRelease(bundle, bundleRelease)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[!] %s\n", err)
			os.Exit(1)
		}

		_, err = certdb.Ensure(rel, tx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[!] %s\n", err)
			os.Exit(1)
		}
	}

	for _, path := range args {
		fileContents, err := ioutil.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[!] %s\n", err)
			os.Exit(1)
		}

		certs, err := helpers.ParseCertificatesPEM(fileContents)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[!] %s\n", err)
			os.Exit(1)
		}

		for _, x509Cert := range certs {
			err := importCertificate(tx, x509Cert, rel)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[!] %s\n", err)
				os.Exit(1)
			}
		}
	}

	db.Close()
}
