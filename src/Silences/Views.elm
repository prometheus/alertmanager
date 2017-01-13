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


silenceList : Silence -> Html Msg
silenceList silence =
    li
        [ class "pa3 pa4-ns bb b--black-10" ]
        [ silenceBase silence
        ]


silence : Silence -> Html Msg
silence silence =
    div []
        [ silenceBase silence
        , silenceExtra silence
        ]


silenceBase : Silence -> Html Msg
silenceBase silence =
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


silenceExtra : Silence -> Html msg
silenceExtra silence =
    let
        -- TODO: It would be nice to get this from the API. I want
        -- Alertmanager's view of things, not the browser client's.
        status =
            "elapsed"
    in
        div [ class "f6" ]
            [ div [ class "mb1" ]
                [ p []
                    [ text "Status: "
                    , Utils.Views.button "ph3 pv2" status
                    ]
                , div []
                    [ label [ class "f6 dib mb2 mr2 w-40" ] [ text "Created by" ]
                    , p [] [ text silence.createdBy ]
                    ]
                , div []
                    [ label [ class "f6 dib mb2 mr2 w-40" ] [ text "Comment" ]
                    , p [] [ text silence.comment ]
                    ]
                ]
            ]



-- TODO: Add field validations.


silenceForm : String -> Silence -> Html Msg
silenceForm kind silence =
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
