module Silences.Views exposing (..)

-- External Imports

import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (onClick, onInput)
import ISO8601


-- Internal Imports

import Types exposing (Silence, Matcher, Msg(..))
import Utils.Views exposing (..)
import Utils.Date


silenceView : Silence -> Html Msg
silenceView silence =
    let
        -- TODO: Check with fabxc if the alert being in the first position can
        -- be relied upon.
        alertName =
            case List.head silence.matchers of
                Just m ->
                    m.value

                Nothing ->
                    ""

        editUrl =
            String.join "/" [ "#/silences", toString silence.id, "edit" ]
    in
        div [ class "f6 mb3" ]
            [ a
                [ class "db link blue mb3"
                , href ("#/silences/" ++ (toString silence.id))
                ]
                [ b [ class "db f4 mb1" ]
                    [ text alertName ]
                ]
            , div [ class "mb1" ]
                [ buttonLink "fa-pencil" editUrl "blue" (Noop [])
                , buttonLink "fa-trash-o" "#/silences" "dark-red" (DestroySilence silence)
                , p [ class "dib mr2" ] [ text <| "Until " ++ Utils.Date.dateFormat silence.endsAt ]
                ]
            , div [ class "mb2 w-80-l w-100-m" ] (List.map matcherButton <| List.filter (\m -> m.name /= "alertname") silence.matchers)
            ]


objectData : String -> Html msg
objectData data =
    dt [ class "m10 black w-100" ] [ text data ]


silenceFormView : String -> Silence -> Html Msg
silenceFormView kind silence =
    let
        base =
            "/#/silences/"

        url =
            case kind of
                "New" ->
                    base ++ "new"

                "Edit" ->
                    base ++ (toString silence.id) ++ "/edit"

                _ ->
                    "/#/silences"
    in
        div [ class "pa4 black-80" ]
            [ fieldset [ class "ba b--transparent ph0 mh0" ]
                [ legend [ class "ph0 mh0 fw6" ] [ text <| kind ++ " Silence" ]
                , formField "Start" (ISO8601.toString silence.startsAt) UpdateStartsAt
                , formField "End" (ISO8601.toString silence.endsAt) UpdateEndsAt
                , div [ class "mt3" ]
                    [ label [ class "f6 b db mb2" ]
                        [ text "Matchers "
                        , span [ class "normal black-60" ] [ text "Alerts affected by this silence." ]
                        ]
                    , label [ class "f6 dib mb2 mr2 w-40" ] [ text "Name" ]
                    , label [ class "f6 dib mb2 mr2 w-40" ] [ text "Value" ]
                    ]
                , div [] <| List.map matcherForm silence.matchers
                , a [ class "f6 link br2 ba ph3 pv2 mr2 dib blue", onClick AddMatcher ] [ text "Add Matcher" ]
                , formField "Creator" silence.createdBy UpdateCreatedBy
                , textField "Comment" silence.comment UpdateComment
                , div [ class "mt3" ]
                    [ a [ class "f6 link br2 ba ph3 pv2 mr2 dib blue", href "#", onClick (CreateSilence silence) ] [ text "Create" ]
                    , a [ class "f6 link br2 ba ph3 pv2 mb2 dib dark-red", href url ] [ text "Reset" ]
                    ]
                ]
            ]


matcherForm : Matcher -> Html Msg
matcherForm matcher =
    div []
        [ input [ class "input-reset ba br1 b--black-20 pa2 mb2 mr2 dib w-40", value matcher.name, onInput (UpdateMatcherName matcher) ] []
        , input [ class "input-reset ba br1 b--black-20 pa2 mb2 mr2 dib w-40", value matcher.value, onInput (UpdateMatcherValue matcher) ] []
        , checkbox "Regex" matcher.isRegex (UpdateMatcherRegex matcher)
        , a [ class <| "f6 link br1 ba mr1 mb2 dib ph1 pv1", onClick (DeleteMatcher matcher) ] [ text "X" ]
        ]


matcherButton : Matcher -> Html msg
matcherButton matcher =
    labelButton ( matcher.name, matcher.value )
