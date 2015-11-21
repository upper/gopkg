package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"
)

var (
	addrFlag       = flag.String("addr", ":8080", "Serve HTTP at given address")
	vanityRootFlag = flag.String("vanity-root", "https://upper.io", "Vanity URL root.")
	repoRootFlag   = flag.String("repo-root", "https://github.com/upper", "Git HTTPs root")
)

var repoRoot *RepoRoot

var urlPattern = regexp.MustCompile(`^/(?:([a-zA-Z0-9][-a-zA-Z0-9]+)/?)?([a-zA-Z][-.a-zA-Z0-9]*)\.?((?:v0|v[1-9][0-9]*)(?:\.0|\.[1-9][0-9]*){0,2}(-unstable)?)(?:\.git)?((?:/[a-zA-Z0-9][-.a-zA-Z0-9]*)*)$`)

var packagePattern = regexp.MustCompile(`^/([a-zA-Z0-9]+)\.?(v[1-9][0-9]*)?(.*)$`)

func main() {
	if err := run(); err != nil {
		log.Fatalf("Could not start server: %q", err)
	}
}

func run() error {
	flag.Parse()

	if *addrFlag == "" {
		return fmt.Errorf("must provide -addr")
	}

	if *repoRootFlag == "" {
		return fmt.Errorf("must provide -repo-root")
	}

	if *vanityRootFlag == "" {
		return fmt.Errorf("must provide -vanity-root")
	}

	var err error
	if repoRoot, err = NewRepoRoot(*repoRootFlag, *vanityRootFlag); err != nil {
		return fmt.Errorf("could not parse -repo-root: %q", err)
	}

	http.HandleFunc("/", handler)

	log.Printf("Listening at %s, proxying to %s", *addrFlag, *repoRootFlag)

	return http.ListenAndServe(*addrFlag, nil)
}

var gogetTemplate = template.Must(template.New("").Parse(`
<html>
<head>
<meta name="go-import" content="{{.GopkgPath}} git http://{{.GopkgPath}}">
{{$root := .RepoRoot}}{{$tree := .GitHubTree}}<meta name="go-source" content="{{.GopkgPath}} _ https://{{$root}}/tree/{{$tree}}{/dir} https://{{$root}}/blob/{{$tree}}{/dir}/{file}#L{line}">
</head>
<body>
go get {{.GopkgPath}}
</body>
</html>
`))

type RepoRoot struct {
	url        *url.URL
	SubPath    string
	VanityPath string
}

func NewRepoRoot(o string, p string) (*RepoRoot, error) {
	u, err := url.Parse(o)
	if err != nil {
		return nil, err
	}
	v, err := url.Parse(p)
	if err != nil {
		return nil, err
	}
	return &RepoRoot{
		url:        u,
		SubPath:    u.Host + u.Path,
		VanityPath: v.Host + v.Path,
	}, nil
}

func (root *RepoRoot) NewRepo(name string) *Repo {
	return &Repo{
		Root:        root,
		Name:        name,
		FullVersion: InvalidVersion,
	}
}

// Repo represents a source code repository on GitHub.
type Repo struct {
	Root *RepoRoot

	Name         string
	MajorVersion Version

	// FullVersion is the best version in AllVersions that matches MajorVersion.
	// It defaults to InvalidVersion if there are no matches.
	FullVersion Version

	// AllVersions holds all versions currently available in the repository,
	// either coming from branch names or from tag names. Version zero (v0)
	// is only present in the list if it really exists in the repository.
	AllVersions VersionList
}

// SetVersions records in the relevant fields the details about which
// package versions are available in the repository.
func (repo *Repo) SetVersions(all []Version) {
	repo.AllVersions = all
	for _, v := range repo.AllVersions {
		if v.Major == repo.MajorVersion.Major && v.Unstable == repo.MajorVersion.Unstable && repo.FullVersion.Less(v) {
			repo.FullVersion = v
		}
	}
}

// RepoRoot returns the repository root at GitHub, without a schema.
func (repo *Repo) RepoRoot() string {
	return repo.Root.SubPath + "/" + repo.Name
}

func (repo *Repo) VanityRoot() string {
	return repo.Root.VanityPath + "/" + repo.Name
}

// GitHubTree returns the repository tree name at GitHub for the selected version.
func (repo *Repo) GitHubTree() string {
	if repo.FullVersion == InvalidVersion {
		return "master"
	}
	return repo.FullVersion.String()
}

// GopkgPath returns the package path at gopkg.in, without a schema.
func (repo *Repo) GopkgPath() string {
	return repo.GopkgVersionRoot(repo.MajorVersion)
}

// GopkgVersionRoot returns the package root in gopkg.in for the
// provided version, without a schema.
func (repo *Repo) GopkgVersionRoot(version Version) string {
	version.Minor = -1
	version.Patch = -1
	v := version.String()
	return repo.VanityRoot() + "." + v
}

func handler(resp http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/health-check" {
		resp.Write([]byte("ok"))
		return
	}

	log.Printf("%s requested %s", req.RemoteAddr, req.URL)

	if req.URL.Path == "/" {
		sendNotFound(resp, "Missing package name.")
		return
	}

	u, err := url.Parse(req.URL.Path)
	if err != nil {
		sendError(resp, "Failed to parse request path")
		return
	}

	p := packagePattern.FindStringSubmatch(u.Path)

	log.Printf("P: %#v", p)

	pkgName := p[1]
	version := p[2]
	extra := p[3]

	repo := repoRoot.NewRepo(pkgName)

	var ok bool
	repo.MajorVersion, ok = parseVersion(version)
	if !ok {
		sendNotFound(resp, "Version %q improperly considered invalid; please warn the service maintainers.", version)
		return
	}

	var changed []byte
	var versions VersionList
	original, err := fetchRefs(repo)
	if err == nil {
		changed, versions, err = changeRefs(original, repo.MajorVersion)
		repo.SetVersions(versions)
	}
	log.Printf("err: %q", err)

	switch err {
	case nil:
		// all ok
	case ErrNoRepo:
		sendNotFound(resp, "Git repository not found at https://%s", repo.RepoRoot())
		return
	case ErrNoVersion:
		major := repo.MajorVersion
		suffix := ""
		if major.Unstable {
			major.Unstable = false
			suffix = unstableSuffix
		}
		v := major.String()
		sendNotFound(resp, `Git repository at https://%s has no branch or tag "%s%s", "%s.N%s" or "%s.N.M%s"`, repo.RepoRoot(), v, suffix, v, suffix, v, suffix)
		return
	default:
		resp.WriteHeader(http.StatusBadGateway)
		resp.Write([]byte(fmt.Sprintf("Cannot obtain refs from Git: %v", err)))
		return
	}

	switch extra {
	case `/git-upload-pack`:
		resp.Header().Set("Location", "https://"+repo.RepoRoot()+"/git-upload-pack")
		resp.WriteHeader(http.StatusMovedPermanently)
		return
	case `/info/refs`:
		resp.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
		resp.Write(changed)
		return
	}

	resp.Header().Set("Content-Type", "text/html")
	if req.FormValue("go-get") == "1" {
		// execute simple template when this is a go-get request
		err = gogetTemplate.Execute(resp, repo)
		if err != nil {
			log.Printf("error executing go get template: %s\n", err)
		}
		return
	}

	sendNotFound(resp, "Missing package name.")
}

func sendError(resp http.ResponseWriter, msg string, args ...interface{}) {
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	resp.WriteHeader(http.StatusInternalServerError)
	resp.Write([]byte(msg))
}

func sendNotFound(resp http.ResponseWriter, msg string, args ...interface{}) {
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	resp.WriteHeader(http.StatusNotFound)
	resp.Write([]byte(msg))
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

const refsSuffix = ".git/info/refs?service=git-upload-pack"

var ErrNoRepo = errors.New("repository not found in GitHub")
var ErrNoVersion = errors.New("version reference not found in GitHub")

func fetchRefs(repo *Repo) (data []byte, err error) {
	repoURL := "https://" + repo.RepoRoot() + refsSuffix
	log.Printf("url: %v", repoURL)
	resp, err := httpClient.Get(repoURL)
	if err != nil {
		return nil, fmt.Errorf("cannot talk to git repository: %v", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200:
		// ok
	case 401, 404:
		return nil, ErrNoRepo
	default:
		return nil, fmt.Errorf("error from git repository: %v", resp.Status)
	}

	data, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading from git: %v", err)
	}
	return data, err
}

func changeRefs(data []byte, major Version) (changed []byte, versions VersionList, err error) {
	var hlinei, hlinej int // HEAD reference line start/end
	var mlinei, mlinej int // master reference line start/end
	var vrefhash string
	var vrefname string
	var vrefv = InvalidVersion

	// Record all available versions, the locations of the master and HEAD lines,
	// and details of the best reference satisfying the requested major version.
	versions = make([]Version, 0)
	sdata := string(data)
	for i, j := 0, 0; i < len(data); i = j {
		size, err := strconv.ParseInt(sdata[i:i+4], 16, 32)
		if err != nil {
			return nil, nil, fmt.Errorf("cannot parse refs line size: %s", string(data[i:i+4]))
		}
		if size == 0 {
			size = 4
		}
		j = i + int(size)
		if j > len(sdata) {
			return nil, nil, fmt.Errorf("incomplete refs data received from GitHub")
		}
		if sdata[0] == '#' {
			continue
		}

		hashi := i + 4
		hashj := strings.IndexByte(sdata[hashi:j], ' ')
		if hashj < 0 || hashj != 40 {
			continue
		}
		hashj += hashi

		namei := hashj + 1
		namej := strings.IndexAny(sdata[namei:j], "\n\x00")
		if namej < 0 {
			namej = j
		} else {
			namej += namei
		}

		name := sdata[namei:namej]

		if name == "HEAD" {
			hlinei = i
			hlinej = j
		}
		if name == "refs/heads/master" {
			mlinei = i
			mlinej = j
		}

		if strings.HasPrefix(name, "refs/heads/v") || strings.HasPrefix(name, "refs/tags/v") {
			if strings.HasSuffix(name, "^{}") {
				// Annotated tag is peeled off and overrides the same version just parsed.
				name = name[:len(name)-3]
			}
			v, ok := parseVersion(name[strings.IndexByte(name, 'v'):])
			if ok && major.Contains(v) && (v == vrefv || !vrefv.IsValid() || vrefv.Less(v)) {
				vrefv = v
				vrefhash = sdata[hashi:hashj]
				vrefname = name
			}
			if ok {
				versions = append(versions, v)
			}
		}
	}

	// If there were absolutely no versions, and v0 was requested, accept the master as-is.
	if len(versions) == 0 && major == (Version{0, -1, -1, false}) {
		return data, nil, nil
	}

	// If the file has no HEAD line or the version was not found, report as unavailable.
	if hlinei == 0 || vrefhash == "" {
		return nil, nil, ErrNoVersion
	}

	var buf bytes.Buffer
	buf.Grow(len(data) + 256)

	// Copy the header as-is.
	buf.Write(data[:hlinei])

	// Extract the original capabilities.
	caps := ""
	if i := strings.Index(sdata[hlinei:hlinej], "\x00"); i > 0 {
		caps = strings.Replace(sdata[hlinei+i+1:hlinej-1], "symref=", "oldref=", -1)
	}

	// Insert the HEAD reference line with the right hash and a proper symref capability.
	var line string
	if strings.HasPrefix(vrefname, "refs/heads/") {
		if caps == "" {
			line = fmt.Sprintf("%s HEAD\x00symref=HEAD:%s\n", vrefhash, vrefname)
		} else {
			line = fmt.Sprintf("%s HEAD\x00symref=HEAD:%s %s\n", vrefhash, vrefname, caps)
		}
	} else {
		if caps == "" {
			line = fmt.Sprintf("%s HEAD\n", vrefhash)
		} else {
			line = fmt.Sprintf("%s HEAD\x00%s\n", vrefhash, caps)
		}
	}
	fmt.Fprintf(&buf, "%04x%s", 4+len(line), line)

	// Insert the master reference line.
	line = fmt.Sprintf("%s refs/heads/master\n", vrefhash)
	fmt.Fprintf(&buf, "%04x%s", 4+len(line), line)

	// Append the rest, dropping the original master line if necessary.
	if mlinei > 0 {
		buf.Write(data[hlinej:mlinei])
		buf.Write(data[mlinej:])
	} else {
		buf.Write(data[hlinej:])
	}

	return buf.Bytes(), versions, nil
}
