module Views.FilterBar.Types exposing (Model, Msg(..), initFilterBar)

import Utils.Filter


type alias Model =
    { matchers : List Utils.Filter.Matcher
    , backspacePressed : Bool
    , matcherText : String
    }


type Msg
    = AddFilterMatcher Bool Utils.Filter.Matcher
    | DeleteFilterMatcher Bool Utils.Filter.Matcher
    | PressingBackspace Bool
    | UpdateMatcherText String
    | Noop


{-| A note about the `backspacePressed` attribute:

Holding down the backspace removes (one by one) each last character in the input,
and the whole time sends multiple keyDown events. This is a guard so that if a user
holds down backspace to remove the text in the input, they won't accidentally hold
backspace too long and then delete the preceding matcher as well. So, once a user holds
backspace to clear an input, they have to then lift up the key and press it again to
proceed to deleting the next matcher.

-}
initFilterBar : List Utils.Filter.Matcher -> Model
initFilterBar matchers =
    { matchers = matchers
    , backspacePressed = False
    , matcherText = ""
    }
