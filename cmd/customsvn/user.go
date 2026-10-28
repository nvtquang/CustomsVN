package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/nvtquang/CustomsVN/internal/auth"
)

// runUser manages accounts from the shell: the way back in when the only
// admin forgot the password.
func runUser(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: customsvn user list | add -email E [-role admin|user] [-name N] [-password P] | passwd -email E [-password P]")
	}
	fs := flag.NewFlagSet("user "+args[0], flag.ContinueOnError)
	configPath := fs.String("config", "", "config file (default ./customsvn.yaml)")
	email := fs.String("email", "", "the account's email ('admin' for the default admin)")
	role := fs.String("role", auth.RoleUser, "admin or user")
	name := fs.String("name", "", "display name")
	password := fs.String("password", "", "password (default: generated and printed)")
	if ok, err := parse(fs, args[1:]); !ok {
		return err
	}
	ctx, cancel := signalContext()
	defer cancel()
	_, pool, err := openDB(ctx, *configPath)
	if err != nil {
		return err
	}
	defer pool.Close()
	store := &auth.Store{Pool: pool}
	pw := *password
	if pw == "" {
		pw = auth.RandomPassword()
	}
	switch args[0] {
	case "list":
		users, err := store.List(ctx)
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tEMAIL\tROLE\tDISABLED\tLAST LOGIN")
		for _, u := range users {
			last := "-"
			if u.LastLogin != nil {
				last = u.LastLogin.Format("2006-01-02 15:04")
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%v\t%s\n", u.ID, u.Email, u.Role, u.Disabled, last)
		}
		return w.Flush()
	case "add":
		u, err := store.Create(ctx, *email, *name, *role, pw)
		if err != nil {
			return err
		}
		fmt.Printf("created %s (%s)\n", u.Email, u.Role)
	case "passwd":
		users, err := store.List(ctx)
		if err != nil {
			return err
		}
		var id int64
		for _, u := range users {
			if u.Email == auth.NormEmail(*email) {
				id = u.ID
			}
		}
		if id == 0 {
			return fmt.Errorf("no user %q", *email)
		}
		enabled := false
		if _, err := store.Update(ctx, id, auth.Update{Password: &pw, Disabled: &enabled}); err != nil {
			return err
		}
		fmt.Printf("password of %s changed; its sessions are closed\n", auth.NormEmail(*email))
	default:
		return fmt.Errorf("unknown user command %q", args[0])
	}
	if *password == "" {
		fmt.Printf("password: %s\n", pw)
	}
	return nil
}
