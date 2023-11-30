#!/bin/bash

set -e

#go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest

protoc --openapiv2_out=disable_default_errors=true,disable_service_tags=true,openapi_naming_strategy=simple,json_names_for_fields=false,allow_merge=true,merge_file_name=api:. ./api.swagger.proto

search='(:\s+{\s+"type":\s)"string",\s+"format":\s"byte"'
replace='\1"object"'

python3 - << EOF
import re

#defining the replace method
def replace(filePath, text, subs, flags=0):
    with open(file_path, "r+") as file:
        #read the file contents
        file_contents = file.read()
        text_pattern = re.compile(text, flags)
        file_contents = text_pattern.sub(subs, file_contents)
        file.seek(0)
        file.truncate()
        file.write(file_contents)

    
file_path="api.swagger.json"
text=r'$search'
subs=r'$replace'
#calling the replace method
replace(file_path, text, subs, flags=re.MULTILINE | re.DOTALL)
EOF

search='(:\s+{\s+"type":\s)"string",\s+"format":\s"uint64"'
replace='\1"integer"'

python3 - << EOF
import re

#defining the replace method
def replace(filePath, text, subs, flags=0):
    with open(file_path, "r+") as file:
        #read the file contents
        file_contents = file.read()
        text_pattern = re.compile(text, flags)
        file_contents = text_pattern.sub(subs, file_contents)
        file.seek(0)
        file.truncate()
        file.write(file_contents)


file_path="api.swagger.json"
text=r'$search'
subs=r'$replace'
#calling the replace method
replace(file_path, text, subs, flags=re.MULTILINE | re.DOTALL)
EOF

search='(:\s+{\s+"type":\s)"string",\s+"format":\s"int64"'
replace='\1"integer"'

python3 - << EOF
import re

#defining the replace method
def replace(filePath, text, subs, flags=0):
    with open(file_path, "r+") as file:
        #read the file contents
        file_contents = file.read()
        text_pattern = re.compile(text, flags)
        file_contents = text_pattern.sub(subs, file_contents)
        file.seek(0)
        file.truncate()
        file.write(file_contents)


file_path="api.swagger.json"
text=r'$search'
subs=r'$replace'
#calling the replace method
replace(file_path, text, subs, flags=re.MULTILINE | re.DOTALL)
EOF
