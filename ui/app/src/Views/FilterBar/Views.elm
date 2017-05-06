module Views.FilterBar.Views exposing (view)

import Html exposing (Html, Attribute, div, span, input, text, button, i, small)
import Html.Attributes exposing (value, class, style, disabled, id)
import Html.Events exposing (onClick, onInput, on, keyCode)
import Utils.Filter exposing (Matcher)
import Views.FilterBar.Types exposing (Msg(..), Model)
import Json.Decode as Json


keys :
    { backspace : Int
    , enter : Int
    }
keys =
    { backspace = 8
    , enter = 13
    }


viewMatcher : Matcher -> Html Msg
viewMatcher matcher =
    div [ class "col col-auto", style [ ( "padding", "5px" ) ] ]
        [ div [ class "btn-group" ]
            [ button
                [ class "btn btn-outline-info"
                , onClick (DeleteFilterMatcher True matcher)
                ]
                [ text <| Utils.Filter.stringifyMatcher matcher
                ]
            , button
                [ class "btn btn-outline-danger"
                , onClick (DeleteFilterMatcher False matcher)
                ]
                [ text "×" ]
            ]
        ]


lastElem : List a -> Maybe a
lastElem =
    List.foldl (Just >> always) Nothing


viewMatchers : List Matcher -> List (Html Msg)
viewMatchers matchers =
    matchers
        |> List.map viewMatcher


onKey : String -> Int -> Msg -> Attribute Msg
onKey event key msg =
    on event
        (Json.map
            (\k ->
                if k == key then
                    msg
                else
                    Noop
            )
            keyCode
        )


view : Model -> Html Msg
view { matchers, matcherText, backspacePressed } =
    let
        className =
            if matcherText == "" then
                ""
            else
                case maybeMatcher of
                    Just _ ->
                        "has-success"

                    Nothing ->
                        "has-danger"

        maybeMatcher =
            Utils.Filter.parseMatcher matcherText

        onKeydown =
            onKey "keydown" keys.backspace <|
                case ( matcherText, backspacePressed ) of
                    ( "", True ) ->
                        Noop

                    ( "", False ) ->
                        lastElem matchers
                            |> Maybe.map (DeleteFilterMatcher True)
                            |> Maybe.withDefault Noop

                    _ ->
                        PressingBackspace True

        onKeypress =
            maybeMatcher
                |> Maybe.map (AddFilterMatcher True)
                |> Maybe.withDefault Noop
                |> onKey "keypress" keys.enter

        onKeyup =
            onKey "keyup" keys.backspace (PressingBackspace False)

        onClickAttr =
            maybeMatcher
                |> Maybe.map (AddFilterMatcher True)
                |> Maybe.withDefault Noop
                |> onClick

        isDisabled =
            maybeMatcher == Nothing
    in
        div
            [ class "row no-gutters align-items-start pb-4" ]
            (viewMatchers matchers
                ++ [ div
                        [ class ("col form-group " ++ className)
                        , style
                            [ ( "padding", "5px" )
                            , ( "min-width", "200px" )
                            , ( "max-width", "500px" )
                            ]
                        ]
                        [ div [ class "input-group" ]
                            [ input
                                [ id "filter-bar-matcher"
                                , class "form-control"
                                , value matcherText
                                , onKeydown
                                , onKeyup
                                , onKeypress
                                , onInput (UpdateMatcherText)
                                ]
                                []
                            , span
                                [ class "input-group-btn" ]
                                [ button [ class "btn btn-primary", disabled isDisabled, onClickAttr ] [ text "Add" ] ]
                            ]
                        , small [ class "form-text text-muted" ]
                            [ text "Custom matcher, e.g."
                            , button
                                [ class "btn btn-link btn-sm align-baseline"
                                , onClick (UpdateMatcherText exampleMatcher)
                                ]
                                [ text exampleMatcher ]
                            ]
                        ]
                   ]
            )


exampleMatcher : String
exampleMatcher =
    "env=\"production\""
