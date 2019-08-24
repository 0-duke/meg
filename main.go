package main

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"sync"
	"time"
	"io/ioutil"
	"path/filepath"
	"strings"
	"github.com/antzucaro/matchr"
)

const (
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/74.0.3729.169 Safari/537.36"

	// argument defaults
	defaultPathsFile = "./paths"
	defaultHostsFile = "./hosts"
	defaultOutputDir = "./out"
)

// a requester is a function that makes HTTP requests
type requester func(request) response

func main() {

	// get the config struct
	c := processArgs()

	// read the paths file
	paths, err := readLinesOrLiteral(c.paths, defaultPathsFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open paths file: %s\n", err)
		os.Exit(1)
	}

	// read the hosts file
	hosts, err := readLinesOrLiteral(c.hosts, defaultHostsFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open hosts file: %s\n", err)
		os.Exit(1)
	}

	// make the output directory
	err = os.MkdirAll(c.output, 0750)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create output directory: %s\n", err)
		os.Exit(1)
	}

	// open the index file
	indexFile := filepath.Join(c.output, "index")
	index, err := os.OpenFile(indexFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open index file for writing: %s\n", err)
		os.Exit(1)
	}

	// set up a rate limiter
	//rl := newRateLimiter(time.Duration(c.delay * 1000000))

	// the request and response channels for
	// the worker pool
	requests := make(chan request)
	responses := make(chan response)

	// spin up some workers to do the requests
	var wg sync.WaitGroup
	for i := 0; i < c.concurrency; i++ {
		wg.Add(1)

		go func() {
			for req := range requests {
				//rl.Block(req.Hostname())
				responses <- c.requester(req)
			}
			wg.Done()
		}()
	}

	// start outputting the response lines; we need a second
	// WaitGroup so we know the outputting has finished
	var owg sync.WaitGroup
	owg.Add(1)
	go func() {
		for res := range responses {
			if len(c.saveStatus) > 0 && !c.saveStatus.Includes(res.statusCode) {
				continue
			}

			if res.err != nil {
				fmt.Fprintf(os.Stderr, "request failed: %s\n", res.err)
				continue
			}
			//Luca: Modify here
			if (true){
				parts := []string{c.output}
				parts = append(parts, res.request.Hostname())
				parts = append(parts, getPathFromStatus(res.status))

				//p := path.Join(parts...)
				p := strings.Join(parts, "/")
				// Read al files in folder with status
				files, err := ioutil.ReadDir(p)
			    if err != nil {
			        fmt.Fprintf(os.Stderr, "failed to read dir: %s\n", err)
			    }

			    res_content := []byte(res.String())
				if c.noHeaders {
					res_content = []byte(res.StringNoHeaders())
				}

			    // For each file, read it and compare it with current response
			    // if not similar save. Otherwise skip
			    new_result := true

			    for _, file := range files {
			        html_from_file, err := ioutil.ReadFile(filepath.Join(p, file.Name()))
			        if err != nil {
			        	fmt.Fprintf(os.Stderr, "File read failed: %s\n", err)
						continue
			        }
			        simil := 0.0
			        res_content_without_headers := excludeHTTPHeaders(res_content)
			        html_from_file_without_headers := excludeHTTPHeaders(html_from_file)

			        //If both HTTP Body are empty there is no reason to proceed with more computational expensive tests 
			        if (len(res_content_without_headers) == 0 && len(html_from_file_without_headers) == 0){
			        	new_result = false
			        	break
			        }
			        // If content is text for one of the two comparisons compare string differences, otherwise compare html
			        if (len(getTagsTokenizer(res_content_without_headers)) == 0 || len(getTagsTokenizer(html_from_file_without_headers)) == 0) {
			        	simil = matchr.Jaro(fmt.Sprintf("%s", res_content_without_headers), fmt.Sprintf("%s", html_from_file_without_headers))
			        } else {
				        simil = similarity(res_content, html_from_file_without_headers)
			        }
			        if c.verbose {
			        	fmt.Printf("%f - %s%s - %s\n", simil, res.request.host, res.request.path, filepath.Join(p, file.Name()))
			        }
			        if(simil >= 0.80) {
			        	new_result = false
			        	break
			        }
			        
			        
			    }
			    line := ""
				if (new_result) {
					fsave_path, err := res.save(c.output, c.noHeaders)
					if err != nil {
						fmt.Fprintf(os.Stderr, "failed to save file: %s\n", err)
					}
					line = fmt.Sprintf("%s %s (%s)\n", fsave_path, res.request.URL(), res.status)
				
				} else {
					line = fmt.Sprintf("%s %s (%s)\n", "NOT-SAVED", res.request.URL(), res.status)
				}

				fmt.Fprintf(index, "%s", line)
				if c.verbose {
					fmt.Printf("%s", line)
				}
			} else {
				fsave_path, err := res.save(c.output, c.noHeaders)
				if err != nil {
					fmt.Fprintf(os.Stderr, "failed to save file: %s\n", err)
				}
				line := fmt.Sprintf("%s %s (%s)\n", fsave_path, res.request.URL(), res.status)
				fmt.Fprintf(index, "%s", line)
				if c.verbose {
					fmt.Printf("%s", line)
				}
			}

			
		}
		owg.Done()
	}()

	// send requests for each path for every host
	for _, path := range paths {
		for _, host := range hosts {

			// the host portion may contain a path prefix,
			// so we should strip that off and add it to
			// the beginning of the path.
			u, err := url.Parse(host)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to parse host: %s\n", err)
				continue
			}
			prefixedPath := u.Path + path
			u.Path = ""

			// stripping off a path means we need to
			// rebuild the host portion too
			host = u.String()

			requests <- request{
				method:         c.method,
				host:           host,
				path:           prefixedPath,
				headers:        c.headers,
				followLocation: c.followLocation,
				timeout:        time.Duration(c.timeout * 1000000),
			}
		}
	}

	// once all of the requests have been sent we can
	// close the requests channel
	close(requests)

	// wait for all the workers to finish before closing
	// the responses channel
	wg.Wait()
	close(responses)

	owg.Wait()

}

// readLines reads all of the lines from a text file in to
// a slice of strings, returning the slice and any error
func readLines(filename string) ([]string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return []string{}, err
	}
	defer f.Close()

	lines := make([]string, 0)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}

	return lines, sc.Err()
}

// readLinesOrLiteral tries to read lines from a file, returning
// the arg in a string slice if the file doesn't exist, unless
// the arg matches its default value
func readLinesOrLiteral(arg, argDefault string) ([]string, error) {
	if isFile(arg) {
		return readLines(arg)
	}

	// if the argument isn't a file, but it is the default, don't
	// treat it as a literal value
	if arg == argDefault {
		return []string{}, fmt.Errorf("file %s not found", arg)
	}

	return []string{arg}, nil
}

// isFile returns true if its argument is a regular file
func isFile(path string) bool {
	f, err := os.Stat(path)
	return err == nil && f.Mode().IsRegular()
}

func getPathFromStatus(status string) string {
	if(len(status) >= 3){
		return status[0:3]
	} else {
		return "XXX"
	}
}
