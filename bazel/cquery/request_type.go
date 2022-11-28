package cquery

import (
	"encoding/json"
	"fmt"
	"strings"
)

var (
	GetOutputFiles      = &getOutputFilesRequestType{}
	GetPythonBinary     = &getPythonBinaryRequestType{}
	GetCcInfo           = &getCcInfoType{}
	GetApexInfo         = &getApexInfoType{}
	GetCcUnstrippedInfo = &getCcUnstippedInfoType{}
)

type CcInfo struct {
	OutputFiles          []string
	CcObjectFiles        []string
	CcSharedLibraryFiles []string
	CcStaticLibraryFiles []string
	Includes             []string
	SystemIncludes       []string
	Headers              []string
	// Archives owned by the current target (not by its dependencies). These will
	// be a subset of OutputFiles. (or static libraries, this will be equal to OutputFiles,
	// but general cc_library will also have dynamic libraries in output files).
	RootStaticArchives []string
	// Dynamic libraries (.so files) created by the current target. These will
	// be a subset of OutputFiles. (or shared libraries, this will be equal to OutputFiles,
	// but general cc_library will also have dynamic libraries in output files).
	RootDynamicLibraries []string
	TidyFiles            []string
	TocFile              string
	UnstrippedOutput     string
}

type getOutputFilesRequestType struct{}

type getPythonBinaryRequestType struct{}

// Name returns a string name for this request type. Such request type names must be unique,
// and must only consist of alphanumeric characters.
func (g getOutputFilesRequestType) Name() string {
	return "getOutputFiles"
}

// StarlarkFunctionBody returns a starlark function body to process this request type.
// The returned string is the body of a Starlark function which obtains
// all request-relevant information about a target and returns a string containing
// this information.
// The function should have the following properties:
//   - The arguments are `target` (a configured target) and `id_string` (the label + configuration).
//   - The return value must be a string.
//   - The function body should not be indented outside of its own scope.
func (g getOutputFilesRequestType) StarlarkFunctionBody() string {
	return "return ', '.join([f.path for f in target.files.to_list()])"
}

// ParseResult returns a value obtained by parsing the result of the request's Starlark function.
// The given rawString must correspond to the string output which was created by evaluating the
// Starlark given in StarlarkFunctionBody.
func (g getOutputFilesRequestType) ParseResult(rawString string) []string {
	return splitOrEmpty(rawString, ", ")
}

// Name returns a string name for this request type. Such request type names must be unique,
// and must only consist of alphanumeric characters.
func (g getPythonBinaryRequestType) Name() string {
	return "getPythonBinary"
}

// StarlarkFunctionBody returns a starlark function body to process this request type.
// The returned string is the body of a Starlark function which obtains
// all request-relevant information about a target and returns a string containing
// this information.
// The function should have the following properties:
//   - The arguments are `target` (a configured target) and `id_string` (the label + configuration).
//   - The return value must be a string.
//   - The function body should not be indented outside of its own scope.
func (g getPythonBinaryRequestType) StarlarkFunctionBody() string {
	return "return providers(target)['FilesToRunProvider'].executable.path"
}

// ParseResult returns a value obtained by parsing the result of the request's Starlark function.
// The given rawString must correspond to the string output which was created by evaluating the
// Starlark given in StarlarkFunctionBody.
func (g getPythonBinaryRequestType) ParseResult(rawString string) string {
	return rawString
}

type getCcInfoType struct{}

// Name returns a string name for this request type. Such request type names must be unique,
// and must only consist of alphanumeric characters.
func (g getCcInfoType) Name() string {
	return "getCcInfo"
}

// StarlarkFunctionBody returns a starlark function body to process this request type.
// The returned string is the body of a Starlark function which obtains
// all request-relevant information about a target and returns a string containing
// this information.
// The function should have the following properties:
//   - The arguments are `target` (a configured target) and `id_string` (the label + configuration).
//   - The return value must be a string.
//   - The function body should not be indented outside of its own scope.
func (g getCcInfoType) StarlarkFunctionBody() string {
	return `
outputFiles = [f.path for f in target.files.to_list()]
p = providers(target)
cc_info = p.get("CcInfo")
if not cc_info:
  fail("%s did not provide CcInfo" % id_string)

includes = cc_info.compilation_context.includes.to_list()
system_includes = cc_info.compilation_context.system_includes.to_list()
headers = [f.path for f in cc_info.compilation_context.headers.to_list()]

ccObjectFiles = []
staticLibraries = []
rootStaticArchives = []
linker_inputs = cc_info.linking_context.linker_inputs.to_list()

static_info_tag = "//build/bazel/rules/cc:cc_library_static.bzl%CcStaticLibraryInfo"
if static_info_tag in p:
  static_info = p[static_info_tag]
  ccObjectFiles = [f.path for f in static_info.objects]
  rootStaticArchives = [static_info.root_static_archive.path]
else:
  for linker_input in linker_inputs:
    for library in linker_input.libraries:
      for object in library.objects:
        ccObjectFiles += [object.path]
      if library.static_library:
        staticLibraries.append(library.static_library.path)
        if linker_input.owner == target.label:
          rootStaticArchives.append(library.static_library.path)

sharedLibraries = []
rootSharedLibraries = []

shared_info_tag = "//build/bazel/rules/cc:cc_library_shared.bzl%CcSharedLibraryOutputInfo"
unstripped_tag = "//build/bazel/rules/cc:stripped_cc_common.bzl%CcUnstrippedInfo"
unstripped = ""

if shared_info_tag in p:
  shared_info = p[shared_info_tag]
  path = shared_info.output_file.path
  sharedLibraries.append(path)
  rootSharedLibraries += [path]
  unstripped = path
  if unstripped_tag in p:
    unstripped = p[unstripped_tag].unstripped.path
else:
  for linker_input in linker_inputs:
    for library in linker_input.libraries:
      if library.dynamic_library:
        path = library.dynamic_library.path
        sharedLibraries.append(path)
        if linker_input.owner == target.label:
          rootSharedLibraries.append(path)

toc_file = ""
toc_file_tag = "//build/bazel/rules/cc:generate_toc.bzl%CcTocInfo"
if toc_file_tag in p:
  toc_file = p[toc_file_tag].toc.path
else:
  # NOTE: It's OK if there's no ToC, as Soong just uses it for optimization
  pass

tidy_files = []
clang_tidy_info = p.get("//build/bazel/rules/cc:clang_tidy.bzl%ClangTidyInfo")
if clang_tidy_info:
  tidy_files = [v.path for v in clang_tidy_info.tidy_files.to_list()]

return json_encode({
	"OutputFiles": outputFiles,
	"CcObjectFiles": ccObjectFiles,
	"CcSharedLibraryFiles": sharedLibraries,
	"CcStaticLibraryFiles": staticLibraries,
	"Includes": includes,
	"SystemIncludes": system_includes,
	"Headers": headers,
	"RootStaticArchives": rootStaticArchives,
	"RootDynamicLibraries": rootSharedLibraries,
	"TidyFiles": tidy_files,
	"TocFile": toc_file,
	"UnstrippedOutput": unstripped,
})`

}

// ParseResult returns a value obtained by parsing the result of the request's Starlark function.
// The given rawString must correspond to the string output which was created by evaluating the
// Starlark given in StarlarkFunctionBody.
func (g getCcInfoType) ParseResult(rawString string) (CcInfo, error) {
	var ccInfo CcInfo
	if err := parseJson(rawString, &ccInfo); err != nil {
		return ccInfo, err
	}
	return ccInfo, nil
}

// Query Bazel for the artifacts generated by the apex modules.
type getApexInfoType struct{}

// Name returns a string name for this request type. Such request type names must be unique,
// and must only consist of alphanumeric characters.
func (g getApexInfoType) Name() string {
	return "getApexInfo"
}

// StarlarkFunctionBody returns a starlark function body to process this request type.
// The returned string is the body of a Starlark function which obtains
// all request-relevant information about a target and returns a string containing
// this information. The function should have the following properties:
//   - The arguments are `target` (a configured target) and `id_string` (the label + configuration).
//   - The return value must be a string.
//   - The function body should not be indented outside of its own scope.
func (g getApexInfoType) StarlarkFunctionBody() string {
	return `
info = providers(target).get("//build/bazel/rules/apex:apex.bzl%ApexInfo")
if not info:
  fail("%s did not provide ApexInfo" % id_string)
bundle_key_info = info.bundle_key_info
container_key_info = info.container_key_info
return json_encode({
    "signed_output": info.signed_output.path,
    "unsigned_output": info.unsigned_output.path,
    "provides_native_libs": [str(lib) for lib in info.provides_native_libs],
    "requires_native_libs": [str(lib) for lib in info.requires_native_libs],
    "bundle_key_info": [bundle_key_info.public_key.path, bundle_key_info.private_key.path],
    "container_key_info": [container_key_info.pem.path, container_key_info.pk8.path, container_key_info.key_name],
    "package_name": info.package_name,
    "symbols_used_by_apex": info.symbols_used_by_apex.path,
    "java_symbols_used_by_apex": info.java_symbols_used_by_apex.path,
    "backing_libs": info.backing_libs.path,
    "bundle_file": info.base_with_config_zip.path,
    "installed_files": info.installed_files.path,
})`
}

type ApexInfo struct {
	SignedOutput          string   `json:"signed_output"`
	UnsignedOutput        string   `json:"unsigned_output"`
	ProvidesLibs          []string `json:"provides_native_libs"`
	RequiresLibs          []string `json:"requires_native_libs"`
	BundleKeyInfo         []string `json:"bundle_key_info"`
	ContainerKeyInfo      []string `json:"container_key_info"`
	PackageName           string   `json:"package_name"`
	SymbolsUsedByApex     string   `json:"symbols_used_by_apex"`
	JavaSymbolsUsedByApex string   `json:"java_symbols_used_by_apex"`
	BackingLibs           string   `json:"backing_libs"`
	BundleFile            string   `json:"bundle_file"`
	InstalledFiles        string   `json:"installed_files"`
}

// ParseResult returns a value obtained by parsing the result of the request's Starlark function.
// The given rawString must correspond to the string output which was created by evaluating the
// Starlark given in StarlarkFunctionBody.
func (g getApexInfoType) ParseResult(rawString string) (ApexInfo, error) {
	var info ApexInfo
	err := parseJson(rawString, &info)
	return info, err
}

// getCcUnstrippedInfoType implements cqueryRequest interface. It handles the
// interaction with `bazel cquery` to retrieve CcUnstrippedInfo provided
// by the` cc_binary` and `cc_shared_library` rules.
type getCcUnstippedInfoType struct{}

func (g getCcUnstippedInfoType) Name() string {
	return "getCcUnstrippedInfo"
}

func (g getCcUnstippedInfoType) StarlarkFunctionBody() string {
	return `unstripped_tag = "//build/bazel/rules/cc:stripped_cc_common.bzl%CcUnstrippedInfo"
p = providers(target)
output_path = target.files.to_list()[0].path
unstripped = output_path
if unstripped_tag in p:
    unstripped = p[unstripped_tag].unstripped.files.to_list()[0].path
return json_encode({
    "OutputFile":  output_path,
    "UnstrippedOutput": unstripped,
})
`
}

// ParseResult returns a value obtained by parsing the result of the request's Starlark function.
// The given rawString must correspond to the string output which was created by evaluating the
// Starlark given in StarlarkFunctionBody.
func (g getCcUnstippedInfoType) ParseResult(rawString string) (CcUnstrippedInfo, error) {
	var info CcUnstrippedInfo
	err := parseJson(rawString, &info)
	return info, err
}

type CcUnstrippedInfo struct {
	OutputFile       string
	UnstrippedOutput string
}

// splitOrEmpty is a modification of strings.Split() that returns an empty list
// if the given string is empty.
func splitOrEmpty(s string, sep string) []string {
	if len(s) < 1 {
		return []string{}
	} else {
		return strings.Split(s, sep)
	}
}

// parseJson decodes json string into the fields of the receiver.
// Unknown attribute name causes panic.
func parseJson(jsonString string, info interface{}) error {
	decoder := json.NewDecoder(strings.NewReader(jsonString))
	decoder.DisallowUnknownFields() //useful to detect typos, e.g. in unit tests
	err := decoder.Decode(info)
	if err != nil {
		return fmt.Errorf("cannot parse cquery result '%s': %s", jsonString, err)
	}
	return nil
}
