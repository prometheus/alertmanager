module Views.AlertList.AlertView exposing (view)

import Alerts.Types exposing (Alert)
import Html exposing (..)
import Html.Attributes exposing (class, style, href)
import Html.Events exposing (onClick)
import Types exposing (Msg(CreateSilenceFromAlert, Noop, MsgForAlertList))
import Utils.Date
import Views.FilterBar.Types as FilterBarTypes
import Views.AlertList.Types exposing (AlertListMsg(MsgForFilterBar))
import Utils.Views exposing (buttonLink)
import Utils.Filter
import Time exposing (Time)


view : Alert -> Html Msg
view alert =
    li
        [ class "align-items-center list-group-item alert-list-item p-0"
        ]
        [ div [ class "d-flex w-100 justify-content-stretch" ]
            [ dateView alert.startsAt
            , labelButtons alert.labels
            , div [ class "d-flex ml-auto" ]
                [ generatorUrlButton alert.generatorUrl
                , silenceButton alert
                ]
            ]
        ]


dateView : Time -> Html Msg
dateView time =
    i
        [ class "d-flex flex-column p-2 text-muted"
        , style
            [ ( "font-family", "monospace" )
            ]
        ]
        [ span [] [ text <| Utils.Date.timeFormat time ]
        , small [] [ text <| Utils.Date.dateFormat time ]
        ]


labelButtons : List ( String, String ) -> Html Msg
labelButtons labels =
    labels
        -- the alertname label should be first
        |>
            List.partition (Tuple.first >> (==) "alertname")
        |> uncurry (++)
        |> List.map labelButton
        |> div [ class "p-2" ]


labelButton : ( String, String ) -> Html Msg
labelButton ( key, value ) =
    let
        msg =
            (FilterBarTypes.AddFilterMatcher False
                { key = key
                , op = Utils.Filter.Eq
                , value = value
                }
                |> MsgForFilterBar
                |> MsgForAlertList
            )
    in
        -- Hide "alertname" key if label is the alertname label
        if key == "alertname" then
            span [ class "pl-2", onClick msg ]
                [ span [ class "badge badge-primary" ]
                    [ i [] [], text value ]
                ]
        else
            Utils.Views.labelButton (Just msg) (key ++ "=" ++ value)


silenceButton : Alert -> Html Msg
silenceButton alert =
    let
        id =
            Maybe.withDefault "" alert.silenceId
    in
        if alert.silenced then
            buttonLink "fa-deaf" ("#/silences/" ++ id) "blue" Noop
        else
            a
                [ class "btn btn-outline-warning rounded-0 border-0 d-flex align-items-center"
                , href "#/silences/new?keep=1"
                , onClick (CreateSilenceFromAlert alert)
                ]
                [ span [ class "fa fa-bell-slash-o" ] [] ]


generatorUrlButton : String -> Html Msg
generatorUrlButton url =
    a
        [ class "btn d-flex btn-outline-primary rounded-0 border-0 align-items-center"
        , href url
        ]
        [ i [ class "fa fa-line-chart" ] [] ]
