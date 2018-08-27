package fix

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

//MiddlewareTransformer moves from our old middleware package to new one
type MiddlewareTransformer struct {
	PackagesReplacement map[string]string
	Aliases             map[string]string
}

func (mw MiddlewareTransformer) transformPackages(r *Runner) error {
	err := filepath.Walk(".", func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if fi.IsDir() {
			return nil
		}

		for _, n := range []string{"vendor", "node_modules", ".git"} {
			if strings.HasPrefix(p, n+string(filepath.Separator)) {
				return nil
			}
		}

		ext := filepath.Ext(p)
		if ext != ".go" {
			return nil
		}

		read, err := ioutil.ReadFile(p)
		if err != nil {
			return err
		}

		newContents := string(read)
		newContents = strings.Replace(newContents, "middleware.SetContentType", "contenttype.Set", -1)
		newContents = strings.Replace(newContents, "middleware.AddContentType", "contenttype.Add", -1)
		newContents = strings.Replace(newContents, "middleware.ParameterLogger", "paramlogger.ParameterLogger", -1)
		newContents = strings.Replace(newContents, "middleware.PopTransaction", "popmw.Transaction", -1)

		err = ioutil.WriteFile(p, []byte(newContents), 0)
		if err != nil {
			return err
		}

		// create an empty fileset.
		fset := token.NewFileSet()

		f, err := parser.ParseFile(fset, p, nil, parser.ParseComments)
		if err != nil {
			e := err.Error()
			msg := "expected 'package', found 'EOF'"
			if e[len(e)-len(msg):] == msg {
				return nil
			}
			return err
		}

		//Replacing mw packages
		for old, new := range mw.PackagesReplacement {
			deleted := astutil.DeleteImport(fset, f, old)
			if deleted {
				astutil.AddNamedImport(fset, f, mw.Aliases[new], new)
			}
		}

		if strings.Contains(newContents, "paramlogger.ParameterLogger") {
			astutil.DeleteImport(fset, f, "github.com/gobuffalo/buffalo/middleware")
			astutil.AddNamedImport(fset, f, "paramlogger", "github.com/gobuffalo/mw-paramlogger")
		}

		if strings.Contains(newContents, "popmw.Transaction") {
			astutil.DeleteImport(fset, f, "github.com/gobuffalo/buffalo/middleware")
			astutil.AddNamedImport(fset, f, "popmw", "github.com/gobuffalo/buffalo-pop/pop/popmw")
		}

		if strings.Contains(newContents, "contenttype.Add") || strings.Contains(newContents, "contenttype.Set") {
			astutil.DeleteImport(fset, f, "github.com/gobuffalo/buffalo/middleware")
			astutil.AddNamedImport(fset, f, "contenttype", "github.com/gobuffalo/mw-contenttype")
		}

		// since the imports changed, resort them.
		ast.SortImports(fset, f)

		// create a temporary file, this easily avoids conflicts.
		temp, err := mw.writeTempResult(p, fset, f)
		if err != nil {
			return err
		}

		// rename the .temp to .go
		return os.Rename(temp, p)
	})
	return err
}

func (mw MiddlewareTransformer) writeTempResult(name string, fset *token.FileSet, f *ast.File) (string, error) {
	temp := name + ".temp"
	w, err := os.Create(temp)
	if err != nil {
		return "", err
	}

	// write changes to .temp file, and include proper formatting.
	err = (&printer.Config{Mode: printer.TabIndent | printer.UseSpaces, Tabwidth: 8}).Fprint(w, fset, f)
	if err != nil {
		return "", err
	}

	// close the writer
	err = w.Close()
	if err != nil {
		return "", err
	}

	return temp, nil
}
