#!/bin/sh
set -e

usage() {
    this=$1
    cat <<EOF
$this: Generate k8s openapi v3 crd definition from Yang model

Usage: $this -r root_node -c crd_node yang_model_file
  -p path used while find yang model imports and the model itself
  -r root_node sets the root container node while parsing the yang model
  -c crd_node sets the crd list node used while parsing the yang model
  -n crd_name is the name of the crd to be generated with openapi v3 k8s schema
  -d output_directory where crd would be generated
  -o enable no config flag for operational status crds
  -t crd_template is the template file to use to generate the crd schema
  -m metadata namespace to generate crd metadata
  -g crd group name
  yang_model_file is the yang model parsed to generate the openapi v3 k8s crd schema
EOF
    exit 2
}

parse_args() {
    OUTPUT_DIR=$(cd $(dirname "$0"); pwd)

    while getopts "h?c:r:p:d:n:ot:m:g:" arg; do
	case "$arg" in
	    p) YANG_MODEL_PATH="$OPTARG" ;;
	    r) ROOT="$OPTARG" ;;
	    c) CRD="$OPTARG" ;;
	    n) CRD_NAME="$OPTARG" ;;
	    d) OUTPUT_DIR="$OPTARG" ;;
            o) NO_CONFIG="-o";;
	    t) CRD_TEMPLATE="$OPTARG";;
	    m) METADATA_NAMESPACE="$OPTARG";;
	    g) CRD_GROUP="$OPTARG";;
	    h | \?) usage "$0" ;;
	esac
    done

    shift $((OPTIND - 1))
    YANG_MODEL=$@
}

validate_args() {
    if [ x"$YANG_MODEL_PATH" = "x" ]; then
	echo "path needs to be specified"
	usage "$0"
    fi
    
    if [ x"$CRD" = "x" ]; then
	echo "crd node would be derived"
    fi

    if [ x"$CRD_TEMPLATE" = "x" ]; then
	echo "crd template needs to be specified"
	usage "$0"
    fi

    if [ x"$ROOT" = "x" ]; then
	echo "root node would be derived"
    fi

    if [ x"$YANG_MODEL" = "x" ]; then
	echo "yang model files need to be specified"
	usage "$0"
    fi

    if [ ! -d $YANG_MODEL_PATH ]; then
	echo "Path $YANG_MODEL_PATH does not exist"
	exit 2
    fi

    if [ ! -f $CRD_TEMPLATE ]; then
	echo "CRD template $CRD_TEMPLATE does not exist"
	exit 2
    fi

    if [ x"$CRD_GROUP" = "x" ]; then
	CRD_GROUP="netconf.ciena.com"
    fi
}

parse_args "$@"
validate_args

paths=$(find $YANG_MODEL_PATH -name "*.yang" -print | xargs -I {} dirname {} | sort | uniq)
filtered_paths=(".git" "test" "RFC" "DRAFT")
module_paths=""
for path in $paths; do
    skip_flag=0
    for f in ${filtered_paths[@]}; do
	if [[ "$path" == *"$f"* ]]; then
	    skip_flag=1
	    break
	fi
    done
    if [ $skip_flag -eq 0 ]; then
	echo "adding $path to yang model search path"
	module_paths="$module_paths $path"
    fi
done

yang_paths=$(echo $module_paths | xargs | sed 's/ /,/g')
if [ x"$CRD_NAME" = "x" ]; then
    echo "Generating crd for $YANG_MODEL with search paths under $YANG_MODEL_PATH, template $CRD_TEMPLATE, root node $ROOT, crd node $CRD, group $CRD_GROUP, output directory $OUTPUT_DIR"
    ./goyang --format crd --ignore-circdep --ignore-resolve-errors --crd-template=$CRD_TEMPLATE --path=$yang_paths -m "$METADATA_NAMESPACE" -r "$ROOT" -c "$CRD" -d $OUTPUT_DIR -u $CRD_GROUP $NO_CONFIG $YANG_MODEL
else
    echo "Generating crd for $YANG_MODEL with search paths under $YANG_MODEL_PATH, template $CRD_TEMPLATE, root node $ROOT, crd node $CRD, crd name $CRD_NAME, group $CRD_GROUP, output directory $OUTPUT_DIR"
    ./goyang --format crd --ignore-circdep --ignore-resolve-errors --crd-template=$CRD_TEMPLATE --path=$yang_paths -m "$METADATA_NAMESPACE" -r "$ROOT" -c "$CRD" -n "$CRD_NAME" -u $CRD_GROUP -d $OUTPUT_DIR $NO_CONFIG $YANG_MODEL
fi
