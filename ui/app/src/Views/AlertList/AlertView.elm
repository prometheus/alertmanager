module Views.AlertList.AlertView exposing (view)

import Alerts.Types exposing (Alert)
import Html exposing (..)
import Html.Attributes exposing (class, style, href)
import Html.Events exposing (onClick)
import Types exposing (Msg(CreateSilenceFromAlert, Noop, MsgForAlertList))
import Utils.Date
import Views.FilterBar.Types as FilterBarTypes
import Views.AlertList.Types exposing (AlertListMsg(MsgForFilterBar, SetActive))
import Utils.Filter


view : List ( String, String ) -> Maybe String -> Alert -> Html Msg
view labels maybeActiveId alert =
    let
        -- remove the grouping labels, and bring the alertname to front
        ungroupedLabels =
            alert.labels
                |> List.filter ((flip List.member) labels >> not)
                |> List.partition (Tuple.first >> (==) "alertname")
                |> uncurry (++)
    in
        li
            [ class "align-items-start list-group-item border-0 alert-list-item p-0 mb-4"
            ]
            [ div
                [ class "w-100 mb-2 d-flex align-items-start" ]
                [ titleView alert
                , if List.length alert.annotations > 0 then
                    annotationsButton maybeActiveId alert
                  else
                    text ""
                , generatorUrlButton alert.generatorUrl
                , silenceButton alert
                ]
            , if maybeActiveId == Just alert.id then
                table [ class "table w-100 mb-1" ] (List.map annotation alert.annotations)
              else
                text ""
            , div [] (List.map labelButton ungroupedLabels)
            ]


titleView : Alert -> Html Msg
titleView { startsAt, isInhibited } =
    let
        ( className, inhibited ) =
            if isInhibited then
                ( "text-muted", " (inhibited)" )
            else
                ( "", "" )
    in
        span
            [ class ("align-self-center mr-2 " ++ className) ]
            [ text
                (Utils.Date.timeFormat startsAt
                    ++ ", "
                    ++ Utils.Date.dateFormat startsAt
                    ++ inhibited
                )
            ]


annotationsButton : Maybe String -> Alert -> Html Msg
annotationsButton maybeActiveId alert =
    if maybeActiveId == Just alert.id then
        button
            [ onClick (SetActive Nothing |> MsgForAlertList)
            , class "btn btn-outline-info border-0 active"
            ]
            [ i [ class "fa fa-minus mr-2" ] [], text "Info" ]
    else
        button
            [ onClick (SetActive (Just alert.id) |> MsgForAlertList)
            , class "btn btn-outline-info border-0"
            ]
            [ i [ class "fa fa-plus mr-2" ] [], text "Info" ]


annotation : ( String, String ) -> Html Msg
annotation ( key, value ) =
    tr []
        [ th [ class "text-nowrap" ] [ text (key ++ ":") ]
        , td [ class "w-100" ] [ text value ]
        ]


labelButton : ( String, String ) -> Html Msg
labelButton ( key, value ) =
    button
        [ class "btn btn-sm bg-faded btn-secondary mr-2 mb-2"
        , onClick (addLabelMsg ( key, value ))
        ]
        [ span [ class "text-muted" ] [ text (key ++ "=\"" ++ value ++ "\"") ] ]


addLabelMsg : ( String, String ) -> Msg
addLabelMsg ( key, value ) =
    (FilterBarTypes.AddFilterMatcher False
        { key = key
        , op = Utils.Filter.Eq
        , value = value
        }
        |> MsgForFilterBar
        |> MsgForAlertList
    )


silenceButton : Alert -> Html Msg
silenceButton alert =
    case alert.silenceId of
        Just sId ->
            a
                [ class "btn btn-outline-danger border-0"
                , href ("#/silences/" ++ sId)
                , onClick (CreateSilenceFromAlert alert)
                ]
                [ i [ class "fa fa-bell-slash mr-2" ] []
                , text "Silenced"
                ]

        Nothing ->
            a
                [ class "btn btn-outline-info border-0"
                , href "#/silences/new?keep=1"
                , onClick (CreateSilenceFromAlert alert)
                ]
                [ i [ class "fa fa-bell-slash-o mr-2" ] []
                , text "Silence"
                ]


generatorUrlButton : String -> Html Msg
generatorUrlButton url =
    a
        [ class "btn btn-outline-info border-0", href url ]
        [ i [ class "fa fa-line-chart mr-2" ] []
        , text "Source"
        ]
