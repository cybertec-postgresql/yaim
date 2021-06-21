## strategy

### healthiness
1. determine healthiness
    - if healthy, continue
    - if not healthy, remove all addresses from interface with label yaim*
        - remove all marks from DCS where name matches ours

### cleanup:
1. get all addresses from interface with label yaim*
2. for each address, check if the address exists in the DCS
    - if not, delete the address from interface
    - if yes, check if the address is marked according to DCS
        - if not marked, register our name in DCS
        - if marked, check registered name
            - if doesn't match our name, delete the address from interface
            - if matches our name, extend TTL

### registration:
1. get all addresses from interface with label yaim*
2. get all addresses from DCS
3. get all healthy yaim from DCS
4. calculate optimum number of addresses per yaim
5. determine if we need to add or delete addresses
    - if need to add: mark address in DCS
        - if marking is successful, add address to interface
            - if failed to add address, remove mark from DCS
    - if need to delete:
        - remove address from interface
            - if successful, remove mark from DCS
