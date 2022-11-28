run_refresh_local() {
	# Test a plain juju refresh with a local charm
	echo

	model_name="test-refresh-local"
	file="${TEST_DIR}/${model_name}.log"
	charm_name="${TEST_DIR}/ubuntu.charm"

	ensure "${model_name}" "${file}"

	juju download ubuntu --no-progress - >"${charm_name}"
	juju deploy "${charm_name}" ubuntu
	wait_for "ubuntu" "$(idle_condition "ubuntu")"

	OUT=$(juju refresh ubuntu --path "${charm_name}" 2>&1 || true)
	if echo "${OUT}" | grep -E -vq "Added local charm"; then
		# shellcheck disable=SC2046
		echo $(red "failed refreshing charm: ${OUT}")
		exit 5
	fi
	# shellcheck disable=SC2059
	printf "${OUT}\n"

	# Added local charm "ubuntu", revision 2, to the model
	revision=$(echo "${OUT}" | awk 'BEGIN{FS=","} {print $2}' | awk 'BEGIN{FS=" "} {print $2}')

	wait_for "ubuntu" "$(charm_rev "ubuntu" "${revision}")"
	wait_for "ubuntu" "$(idle_condition "ubuntu")"

	destroy_model "${model_name}"
}

test_basic() {
	if [ "$(skip 'test_basic')" ]; then
		echo "==> TEST SKIPPED: basic refresh"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_refresh_local"
	)
}
