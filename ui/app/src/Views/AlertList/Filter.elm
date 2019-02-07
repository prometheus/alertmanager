module Views.AlertList.Filter exposing (matchers)

import Alerts.Types exposing (Alert, AlertGroup, Block)
import Regex exposing (contains, regex)
import Utils.Types exposing (Matchers)


matchers : Maybe Utils.Types.Matchers -> List AlertGroup -> List AlertGroup
matchers matchers alerts =
    case matchers of
        Just ms ->
            by (filterAlertGroupLabels ms) alerts

        Nothing ->
            alerts


alertsFromBlock : (Alert -> Bool) -> Block -> Maybe Block
alertsFromBlock fn block =
    let
        alerts =
            List.filter fn block.alerts
    in
    if not <| List.isEmpty alerts then
        Just { block | alerts = alerts }

    else
        Nothing


byLabel : Utils.Types.Matchers -> Block -> Maybe Block
byLabel matchers block =
    alertsFromBlock
        (\a ->
            -- Check that all labels are present within the alert's label set.
            List.all
                (\m ->
                    -- Check for regex or direct match
                    if m.isRegex then
                        -- Check if key is present, then regex match value.
                        let
                            x =
                                List.head <| List.filter (\( key, value ) -> key == m.name) a.labels

                            regex =
                                Regex.regex m.value
                        in
                        -- No regex match
                        case x of
                            Just ( _, value ) ->
                                Regex.contains regex value

                            Nothing ->
                                False

                    else
                        List.member ( m.name, m.value ) a.labels
                )
                matchers
        )
        block


filterAlertGroupLabels : Utils.Types.Matchers -> AlertGroup -> Maybe AlertGroup
filterAlertGroupLabels matchers alertGroup =
    let
        blocks =
            List.filterMap (byLabel matchers) alertGroup.blocks
    in
    if not <| List.isEmpty blocks then
        Just { alertGroup | blocks = blocks }

    else
        Nothing


by : (a -> Maybe a) -> List a -> List a
by fn groups =
    List.filterMap fn groups
