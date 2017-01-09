module Silences.Views exposing (..)

-- External Imports

import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (onClick, onInput)


-- Internal Imports

import Types exposing (Silence, Matcher, Msg(..))
import Utils.Views exposing (..)


silenceView : Silence -> Html msg
silenceView silence =
    let
        dictMatchers =
            List.map (\x -> ( x.name, x.value )) silence.matchers
    in
        div []
            [ dl [ class "mt2 f6 lh-copy" ]
                [ objectData (toString silence.id)
                , objectData silence.createdBy
                , objectData silence.comment
                ]
            , ul [ class "list" ]
                (List.map labelButton dictMatchers)
            , a
                [ class "f6 link br2 ba ph3 pv2 mr2 dib dark-blue"
                , href ("#/silences/" ++ (toString silence.id) ++ "/edit")
                ]
                [ text "Create" ]
            ]


objectData : String -> Html msg
objectData data =
    dt [ class "m10 black w-100" ] [ text data ]



-- Start
-- End
-- Matchers (Name, Value, Regexp) and add additional
-- Creator
-- Comment
-- Create


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
                , formField "Start" silence.startsAt UpdateStartsAt
                , formField "End" silence.endsAt UpdateEndsAt
                , div [ class "mt3" ]
                    [ label [ class "f6 b db mb2" ]
                        [ text "Matchers "
                        , span [ class "normal black-60" ] [ text "Alerts affected by this silence." ]
                        ]
                    , label [ class "f6 dib mb2 mr2 w-40" ] [ text "Name" ]
                    , label [ class "f6 dib mb2 mr2 w-40" ] [ text "Value" ]
                    ]
                , div [] <| List.map matcherForm silence.matchers
                , a [ class "f6 link br2 ba ph3 pv2 mr2 dib dark-blue", onClick AddMatcher ] [ text "Add Matcher" ]
                , formField "Creator" silence.createdBy UpdateCreatedBy
                , textField "Comment" silence.comment UpdateComment
                , div [ class "mt3" ]
                    [ a [ class "f6 link br2 ba ph3 pv2 mr2 dib dark-blue", href "#" ] [ text "Create" ]
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
